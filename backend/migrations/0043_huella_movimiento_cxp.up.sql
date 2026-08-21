-- Huella Bancos↔CxP: rastro del movimiento bancario que ES el pago de una factura.
--
-- Hasta ahora la conciliación por huella cambiaba el estado del documento pero no dejaba
-- constancia de CUÁL movimiento lo pagó. Sin ese enlace no hay idempotencia (un segundo
-- barrido reintenta) ni trazabilidad (nadie puede ir del banco a la factura y volver).
ALTER TABLE movimiento_bancario
    ADD COLUMN documento_cxp_id uuid REFERENCES documento_cxp(id);

-- Un movimiento paga una factura y una factura se paga con un movimiento: el enlace es 1-1.
-- (Un pago agrupado de varias facturas se registra por su lote, no por este enlace.)
CREATE UNIQUE INDEX idx_mov_documento_cxp
    ON movimiento_bancario (documento_cxp_id) WHERE documento_cxp_id IS NOT NULL;

CREATE INDEX idx_mov_huella_pendiente
    ON movimiento_bancario (empresa_id) WHERE documento_cxp_id IS NULL;
