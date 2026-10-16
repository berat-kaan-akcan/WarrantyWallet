-- V2: service history — repair cost, resolution time and a status timeline.
ALTER TABLE service_records ADD COLUMN cost numeric(12,2) NOT NULL DEFAULT 0 CHECK (cost >= 0);
ALTER TABLE service_records ADD COLUMN resolved_at timestamptz;

CREATE TABLE service_events (
    id                uuid PRIMARY KEY,
    service_record_id uuid NOT NULL REFERENCES service_records(id) ON DELETE CASCADE,
    status            text NOT NULL CHECK (status IN ('open', 'in_service', 'resolved', 'cancelled')),
    note              text NOT NULL DEFAULT '',
    created_at        timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX service_events_record_idx ON service_events (service_record_id, created_at);
