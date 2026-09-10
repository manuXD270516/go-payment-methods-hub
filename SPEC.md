# SPEC — go-payment-methods-hub
### Hub de métodos de pago, tokenización y proveedores · Spec-Driven Development

> Esta especificación define **qué** debe construirse y **cómo se verifica**. El agente implementa contra ella; los criterios de aceptación son el contrato.

## 1. Propósito

Servicio en Go que unifica los **métodos de pago** de una pasarela detrás de una interfaz común: tarjeta (con tokenización), transferencia bancaria, efectivo y QR. Enruta cada operación al **proveedor externo** adecuado según método, país y reglas de negocio, normalizando contratos heterogéneos.

Complementa a `go-payments-core`: el core maneja el ciclo transaccional y la contabilidad; este hub maneja *cómo* se cobra y *con quién*.

## 2. Alcance

### Entra
- **Catálogo de métodos de pago** por comercio y país, con habilitación y reglas.
- **Tokenización de tarjetas**: sustitución del PAN por un token opaco; el PAN nunca se persiste en la base del hub.
- **Adaptadores de proveedores** (`Provider`): interfaz común, implementaciones intercambiables (mock + al menos dos simulados de países distintos).
- **Enrutamiento**: selección de proveedor por método, país, moneda y disponibilidad, con **fallback** ante fallo.
- **Cash-in / cash-out**: flujos de recaudo y de pago a terceros.
- **Circuit breaker + retry con backoff** por proveedor.
- **Normalización de errores** de proveedores a un catálogo propio de códigos.
- Webhooks entrantes de proveedores con **verificación de firma** e idempotencia.

### No entra
- Contabilidad y ciclo de vida transaccional (eso es `go-payments-core`).
- PCI-DSS real de almacenamiento de PAN: el vault se implementa como **puerto** con adaptador simulado (cifrado local con clave de entorno), documentando que en producción iría un HSM/vault certificado.

## 3. Modelo de dominio

- **PaymentMethod**: `type (card|bank_transfer|cash|qr)`, `country`, `currency`, `enabled`, `merchant_id`, `rules`.
- **CardToken**: `token`, `brand`, `last4`, `exp_month`, `exp_year`, `fingerprint`, `merchant_id`. **Sin PAN.**
- **Provider**: `id`, `name`, `countries`, `methods`, `priority`, `health`.
- **RoutingDecision**: método + país + moneda → proveedor elegido, con motivo y alternativas.
- **ProviderOperation**: `charge`, `payout`, `status`, `refund` — resultado normalizado.

## 4. Reglas de negocio (invariantes)

1. El PAN se recibe, se tokeniza y **se descarta**; nunca se persiste ni se loguea.
2. Un token pertenece a un comercio; otro comercio no puede usarlo (aislamiento multitenant).
3. El enrutamiento es **determinístico y explicable**: dada la misma entrada, el mismo proveedor, y la decisión queda registrada con su motivo.
4. Si el proveedor primario falla y hay alternativa elegible, se hace fallback; el intento fallido queda registrado.
5. Un proveedor con circuito abierto no recibe tráfico hasta pasar el periodo de prueba.
6. Los webhooks entrantes con firma inválida se rechazan con `401` y se registran.
7. Un webhook repetido (mismo `event_id`) no re-ejecuta efectos.
8. Los errores de proveedores se normalizan a códigos propios; el consumidor nunca ve el error crudo del tercero.

## 5. API (contratos)

| Método | Ruta | Descripción |
|---|---|---|
| GET | `/v1/payment-methods` | Métodos disponibles por comercio/país |
| POST | `/v1/tokens` | Tokeniza una tarjeta (devuelve token + last4 + brand) |
| GET | `/v1/tokens/{token}` | Metadatos del token (nunca el PAN) |
| DELETE | `/v1/tokens/{token}` | Revoca un token |
| POST | `/v1/charges` | Cobro (cash-in) por método y token/instrucción |
| POST | `/v1/payouts` | Pago a tercero (cash-out) |
| GET | `/v1/routing/preview` | Explica qué proveedor se elegiría y por qué |
| POST | `/v1/webhooks/{provider}` | Recepción de notificaciones con firma verificada |
| GET | `/healthz` `/readyz` | Health y readiness |

Errores en **RFC 7807** con códigos de dominio normalizados.

## 6. Arquitectura

Hexagonal / Clean, estricta:

```
cmd/api                          → composición y arranque
internal/domain/{method,token,provider} → entidades, reglas de enrutamiento. Sin deps externas
internal/application/usecase     → casos de uso; dependen de PUERTOS
internal/infrastructure/providers→ adaptadores por proveedor (uno por archivo)
internal/infrastructure/vault    → adaptador de tokenización/cifrado
internal/infrastructure/{http,persistence} → entrada HTTP y Postgres
```

**Regla clave:** agregar un proveedor nuevo = crear un adaptador que implemente la interfaz `Provider`, **sin tocar el dominio ni los casos de uso**. Esto se verifica con un test.

## 7. Criterios de aceptación (verificables)

- [ ] `make up && make migrate && make run` levanta el servicio.
- [ ] `POST /v1/tokens` devuelve token, `brand` y `last4`; el PAN **no** aparece en la base ni en los logs (test de redacción y de esquema).
- [ ] Un token creado por el comercio A no puede usarse por el comercio B → `404`/`403`.
- [ ] `GET /v1/routing/preview` explica la decisión: proveedor elegido, motivo y alternativas.
- [ ] Con el proveedor primario forzado a fallar, el cobro cae al secundario y ambos intentos quedan registrados.
- [ ] Tras N fallos consecutivos, el circuito del proveedor abre y deja de recibir tráfico.
- [ ] Webhook con firma inválida → `401`; con firma válida y `event_id` repetido → efecto ejecutado **una sola vez**.
- [ ] Agregar un proveedor nuevo no requiere modificar `domain` ni `application` (test de arquitectura).
- [ ] Errores de proveedor normalizados: dos proveedores distintos con "fondos insuficientes" devuelven el **mismo** código de dominio.
- [ ] `go test ./... -race` en verde; cobertura de `domain` ≥ 80%.
- [ ] `go vet ./...` y `golangci-lint run` sin hallazgos.

## 8. Stack

Go 1.22+ · chi · PostgreSQL (pgx) · migraciones versionadas · `testcontainers-go` · `httptest` para simular proveedores · circuit breaker propio o `gobreaker` · `slog` · Docker + compose.
