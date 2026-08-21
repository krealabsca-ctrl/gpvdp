DROP INDEX IF EXISTS idx_mov_huella_pendiente;
DROP INDEX IF EXISTS idx_mov_documento_cxp;
ALTER TABLE movimiento_bancario DROP COLUMN IF EXISTS documento_cxp_id;
