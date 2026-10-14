DROP INDEX IF EXISTS idx_contrato_cxc_proximo_pago;
ALTER TABLE contrato_cxc DROP COLUMN IF EXISTS fecha_proximo_pago;
