-- 0001_init: core schema for Warranty Wallet V1
CREATE EXTENSION IF NOT EXISTS citext;

CREATE TABLE users (
    id            uuid PRIMARY KEY,
    email         citext NOT NULL UNIQUE,
    password_hash text   NOT NULL,
    name          text   NOT NULL DEFAULT '',
    role          text   NOT NULL DEFAULT 'user' CHECK (role IN ('user', 'admin', 'business')),
    created_at    timestamptz NOT NULL DEFAULT now()
);

CREATE TABLE categories (
    slug    text PRIMARY KEY,
    name_tr text NOT NULL
);

INSERT INTO categories (slug, name_tr) VALUES
    ('phone', 'Telefon'), ('laptop', 'Laptop'), ('monitor', 'Monitör'), ('tv', 'Televizyon'),
    ('headphones', 'Kulaklık'), ('tablet', 'Tablet'), ('appliance', 'Beyaz Eşya'),
    ('small_appliance', 'Küçük Ev Aleti'), ('camera', 'Kamera'), ('console', 'Oyun Konsolu'),
    ('wearable', 'Akıllı Saat'), ('other', 'Diğer');

CREATE TABLE products (
    id              uuid PRIMARY KEY,
    user_id         uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    name            text NOT NULL,
    brand           text NOT NULL DEFAULT '',
    model           text NOT NULL DEFAULT '',
    category        text NOT NULL DEFAULT 'other',
    seller          text NOT NULL DEFAULT '',
    purchase_date   date NOT NULL,
    purchase_price  numeric(12, 2) NOT NULL DEFAULT 0 CHECK (purchase_price >= 0),
    currency        char(3) NOT NULL DEFAULT 'TRY',
    warranty_months integer NOT NULL DEFAULT 0 CHECK (warranty_months BETWEEN 0 AND 240),
    warranty_end    date,
    serial_number   text NOT NULL DEFAULT '',
    notes           text NOT NULL DEFAULT '',
    created_at      timestamptz NOT NULL DEFAULT now(),
    updated_at      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX products_user_created_idx ON products (user_id, created_at DESC);
CREATE INDEX products_user_end_idx ON products (user_id, warranty_end);

CREATE TABLE service_records (
    id             uuid PRIMARY KEY,
    user_id        uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id     uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    problem        text NOT NULL CHECK (problem IN ('wont_turn_on', 'screen_lines', 'physical_damage', 'other')),
    description    text NOT NULL DEFAULT '',
    service_name   text NOT NULL DEFAULT '',
    service_date   date,
    status         text NOT NULL DEFAULT 'open' CHECK (status IN ('open', 'in_service', 'resolved', 'cancelled')),
    notes          text NOT NULL DEFAULT '',
    under_warranty boolean NOT NULL DEFAULT false,
    created_at     timestamptz NOT NULL DEFAULT now(),
    updated_at     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX service_records_user_idx ON service_records (user_id, created_at DESC);

CREATE TABLE documents (
    id                uuid PRIMARY KEY,
    user_id           uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id        uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    service_record_id uuid REFERENCES service_records(id) ON DELETE SET NULL,
    type              text NOT NULL CHECK (type IN ('invoice', 'warranty', 'service', 'photo')),
    file_key          text NOT NULL,
    content_type      text NOT NULL DEFAULT 'application/octet-stream',
    size_bytes        bigint NOT NULL DEFAULT 0,
    original_name     text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX documents_product_idx ON documents (product_id, created_at);

CREATE TABLE notifications (
    id           uuid PRIMARY KEY,
    user_id      uuid NOT NULL REFERENCES users(id) ON DELETE CASCADE,
    product_id   uuid NOT NULL REFERENCES products(id) ON DELETE CASCADE,
    type         text NOT NULL,
    message      text NOT NULL DEFAULT '',
    scheduled_at timestamptz NOT NULL,
    sent         boolean NOT NULL DEFAULT false,
    sent_at      timestamptz,
    UNIQUE (product_id, type)
);
CREATE INDEX notifications_due_idx ON notifications (scheduled_at) WHERE NOT sent;

CREATE TABLE extraction_logs (
    id         uuid PRIMARY KEY,
    user_id    uuid REFERENCES users(id) ON DELETE SET NULL,
    provider   text NOT NULL,
    success    boolean NOT NULL,
    error      text NOT NULL DEFAULT '',
    raw_text   text NOT NULL DEFAULT '',
    created_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX extraction_logs_created_idx ON extraction_logs (created_at DESC);
