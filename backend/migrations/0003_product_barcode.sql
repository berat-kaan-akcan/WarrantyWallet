-- 0003_product_barcode: EAN/UPC/QR code scanned from the product or its box (V2 scanning)
ALTER TABLE products ADD COLUMN barcode text NOT NULL DEFAULT '';
CREATE INDEX products_user_barcode_idx ON products (user_id, lower(barcode)) WHERE barcode <> '';
CREATE INDEX products_user_serial_idx ON products (user_id, lower(serial_number)) WHERE serial_number <> '';
