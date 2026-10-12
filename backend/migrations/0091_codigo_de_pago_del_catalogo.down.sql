-- Deshace la nomenclatura interna del pago.
DROP INDEX IF EXISTS idx_clasificacion_codigo_pago;
DROP INDEX IF EXISTS idx_concepto_codigo_pago;
ALTER TABLE clasificacion DROP CONSTRAINT IF EXISTS clasificacion_codigo_pago_check;
ALTER TABLE concepto DROP CONSTRAINT IF EXISTS concepto_codigo_pago_check;
ALTER TABLE clasificacion DROP COLUMN IF EXISTS codigo_pago;
ALTER TABLE concepto DROP COLUMN IF EXISTS codigo_pago;
