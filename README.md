# go-payment-methods-hub

Hub de métodos de pago para una pasarela: unifica tarjeta, transferencia, efectivo y QR detrás de una interfaz común, tokeniza tarjetas y enruta cada operación al proveedor externo adecuado según método, país y disponibilidad.

Complementa a `go-payments-core`: el core lleva el ciclo transaccional y la contabilidad; este hub resuelve *cómo* se cobra y *con quién*.

## Por qué existe

Integrar proveedores de pago es un problema de contratos heterogéneos: cada uno tiene su formato, sus errores y sus caídas. Este servicio normaliza esa diversidad y la hace resistente a fallos, de modo que sumar un proveedor sea agregar un adaptador y no reescribir el sistema.

## Características

- Tokenización de tarjetas: el PAN se recibe, se tokeniza y se descarta (nunca persistido ni logueado)
- Adaptadores de proveedor intercambiables tras una interfaz común
- Enrutamiento determinístico y **explicable** (devuelve el motivo y las alternativas)
- Resiliencia: retry con backoff y jitter, circuit breaker por proveedor y fallback
- Cash-in y cash-out con proveedores de distintos países
- Webhooks entrantes con verificación de firma e idempotencia por `event_id`
- Errores de terceros normalizados a un catálogo propio de códigos

## Stack

Go 1.22+ · PostgreSQL (pgx) · chi · testcontainers-go · httptest para simular proveedores · slog · Docker

## Arranque rápido

```bash
make up && make migrate && make run
make test
```

## Documentación

- [`SPEC.md`](SPEC.md) — especificación y criterios de aceptación
- [`ARCHITECTURE.md`](ARCHITECTURE.md) — capas, puertos y decisiones
- [`ARCHITECT-PROMPT.md`](ARCHITECT-PROMPT.md) — prompt de arquitecto para desarrollo asistido

## Estado

En construcción, por etapas, siguiendo `SPEC.md`.
