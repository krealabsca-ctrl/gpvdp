-- Se borran las FACTURAS adjuntas antes de volver atrás: sin ellas el UNIQUE por documento no se
-- puede recrear (una factura con sus dos documentos daría dos filas con el mismo documento_id).
DELETE FROM comprobante_pago WHERE tipo = 'FACTURA';

DROP INDEX IF EXISTS ux_adjunto_documento_tipo;

ALTER TABLE comprobante_pago
    ADD CONSTRAINT comprobante_pago_documento_id_key UNIQUE (documento_id);

ALTER TABLE comprobante_pago
    DROP CONSTRAINT IF EXISTS comprobante_pago_tipo_check,
    DROP COLUMN IF EXISTS tipo;
