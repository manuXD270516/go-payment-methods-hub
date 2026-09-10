# ARCHITECTURE — go-payment-methods-hub

## Capas y regla de dependencia

```
        cmd/api  (composición y arranque)
             │
   internal/infrastructure  (providers, vault, http, postgres)
             │  implementa los puertos
   internal/application/usecase  (casos de uso; dependen de PUERTOS)
             │  usa
   internal/domain  (método, token, proveedor, enrutamiento) ← sin deps externas
```

## Puertos

- `Vault` — tokeniza y recupera metadatos; **nunca** devuelve el PAN.
- `ProviderPort` — `Charge`, `Payout`, `Status`, `Refund` con contrato normalizado.
- `MethodRepository`, `TokenRepository` — persistencia.
- `RoutingPolicy` — decide proveedor y explica el motivo.
- `EventPublisher` — emite eventos hacia `go-payments-core`.

## Decisiones de arquitectura (ADR resumidas)

**ADR-001 — Tokenización con vault como puerto.** El dominio conoce tokens, no tarjetas. El adaptador local cifra con clave de entorno; en producción se sustituye por HSM/vault certificado sin tocar el dominio.

**ADR-002 — Un adaptador por proveedor.** Cada integración vive aislada e implementa `ProviderPort`. Sumar un proveedor no modifica dominio ni casos de uso (verificado con test de arquitectura).

**ADR-003 — Enrutamiento explicable.** La decisión no es una caja negra: devuelve proveedor elegido, motivo y alternativas. Facilita soporte y auditoría.

**ADR-004 — Resiliencia por proveedor.** Retry con backoff exponencial y jitter, circuit breaker por proveedor, y fallback a alternativa elegible.

**ADR-005 — Normalización de errores.** Catálogo propio de códigos; los errores de terceros se mapean. El consumidor programa contra un contrato estable.

**ADR-006 — Webhooks con firma e idempotencia.** Verificación de firma obligatoria y deduplicación por `event_id` para evitar efectos repetidos.

## Modelo de datos (resumen)

- `payment_methods` — merchant_id, type, country, currency, enabled, rules.
- `card_tokens` — token, merchant_id, brand, last4, exp_month, exp_year, fingerprint. **Sin PAN.**
- `providers` — id, name, countries, methods, priority, health_state.
- `routing_decisions` — input, provider_id, reason, alternatives, created_at.
- `provider_operations` — id, provider_id, type, external_ref, normalized_code, attempts.
- `webhook_events` — provider, event_id (único), signature_valid, processed_at.

## Seguridad

Sin PAN en base ni logs; redacción activa en el logger; firma verificada en webhooks; secretos por entorno; timeouts obligatorios en llamadas salientes; aislamiento multitenant en toda consulta.
