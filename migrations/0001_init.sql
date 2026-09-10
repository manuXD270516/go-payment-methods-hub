-- Esquema inicial del hub de métodos de pago.
-- Obsérvese que no existe ninguna columna para el número de tarjeta: el PAN
-- vive cifrado en el vault, fuera de esta base.

CREATE TABLE IF NOT EXISTS card_tokens (
    token        TEXT        PRIMARY KEY,
    merchant_id  TEXT        NOT NULL,
    brand        TEXT        NOT NULL,
    last4        CHAR(4)     NOT NULL,
    exp_month    SMALLINT    NOT NULL CHECK (exp_month BETWEEN 1 AND 12),
    exp_year     SMALLINT    NOT NULL,
    fingerprint  TEXT        NOT NULL,
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now(),
    revoked_at   TIMESTAMPTZ
);

CREATE INDEX IF NOT EXISTS idx_tokens_merchant     ON card_tokens (merchant_id, token);
CREATE INDEX IF NOT EXISTS idx_tokens_fingerprint  ON card_tokens (merchant_id, fingerprint);

CREATE TABLE IF NOT EXISTS providers (
    id           TEXT        PRIMARY KEY,
    name         TEXT        NOT NULL,
    countries    TEXT[]      NOT NULL,
    methods      TEXT[]      NOT NULL,
    currencies   TEXT[]      NOT NULL,
    priority     INTEGER     NOT NULL DEFAULT 100,
    health       TEXT        NOT NULL DEFAULT 'healthy',
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS payment_methods (
    id           BIGSERIAL   PRIMARY KEY,
    merchant_id  TEXT        NOT NULL,
    type         TEXT        NOT NULL,
    country      CHAR(2)     NOT NULL,
    currency     CHAR(3)     NOT NULL,
    enabled      BOOLEAN     NOT NULL DEFAULT TRUE,
    rules        JSONB       NOT NULL DEFAULT '{}'::jsonb,
    UNIQUE (merchant_id, type, country, currency)
);

-- Rastro de decisiones: permite explicar a soporte por qué se eligió un proveedor.
CREATE TABLE IF NOT EXISTS routing_decisions (
    id           BIGSERIAL   PRIMARY KEY,
    merchant_id  TEXT        NOT NULL,
    method       TEXT        NOT NULL,
    country      CHAR(2)     NOT NULL,
    currency     CHAR(3)     NOT NULL,
    provider_id  TEXT        NOT NULL,
    reason       TEXT        NOT NULL,
    alternatives TEXT[],
    created_at   TIMESTAMPTZ NOT NULL DEFAULT now()
);

CREATE TABLE IF NOT EXISTS provider_operations (
    id              TEXT        PRIMARY KEY,
    merchant_id     TEXT        NOT NULL,
    provider_id     TEXT        NOT NULL,
    type            TEXT        NOT NULL CHECK (type IN ('charge', 'payout')),
    amount          BIGINT      NOT NULL CHECK (amount > 0),
    currency        CHAR(3)     NOT NULL,
    external_ref    TEXT,
    normalized_code TEXT,
    attempts        INTEGER     NOT NULL DEFAULT 1,
    created_at      TIMESTAMPTZ NOT NULL DEFAULT now()
);

-- La unicidad de event_id garantiza que un webhook repetido no repita efectos.
CREATE TABLE IF NOT EXISTS webhook_events (
    provider_id     TEXT        NOT NULL,
    event_id        TEXT        NOT NULL,
    signature_valid BOOLEAN     NOT NULL,
    processed_at    TIMESTAMPTZ,
    received_at     TIMESTAMPTZ NOT NULL DEFAULT now(),
    PRIMARY KEY (provider_id, event_id)
);
