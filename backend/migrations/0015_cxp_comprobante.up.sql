-- CxP: comprobante de pago adjunto por factura (PDF en BD) + marca de envío al proveedor.
-- Un comprobante por documento (se reemplaza si se vuelve a subir).

CREATE TABLE comprobante_pago (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id   uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    documento_id uuid NOT NULL REFERENCES documento_cxp(id) ON DELETE CASCADE,
    filename     text NOT NULL,
    mime         text NOT NULL DEFAULT 'application/pdf',
    contenido    bytea NOT NULL,
    subido_por   uuid REFERENCES usuario(id),
    subido_en    timestamptz NOT NULL DEFAULT now(),
    UNIQUE (documento_id)
);
CREATE INDEX idx_comprobante_empresa ON comprobante_pago (empresa_id);

ALTER TABLE documento_cxp ADD COLUMN comprobante_enviado_en timestamptz;
