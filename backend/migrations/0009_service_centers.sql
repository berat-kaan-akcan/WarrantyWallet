-- 0009_service_centers: authorized service centers maintained by admins (service-center finder)
CREATE TABLE service_centers (
    id         uuid PRIMARY KEY,
    brand      text NOT NULL,
    brand_key  text NOT NULL, -- folded for matching (see domain.FoldKey)
    name       text NOT NULL,
    city       text NOT NULL,
    city_key   text NOT NULL,
    district   text NOT NULL DEFAULT '',
    address    text NOT NULL DEFAULT '',
    phone      text NOT NULL DEFAULT '',
    website    text NOT NULL DEFAULT '',
    lat        double precision,
    lng        double precision,
    created_at timestamptz NOT NULL DEFAULT now(),
    updated_at timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX service_centers_brand_city_idx ON service_centers (brand_key, city_key);
