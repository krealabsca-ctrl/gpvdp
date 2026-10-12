-- Deshace el enlace movimiento → remesa.
DROP INDEX IF EXISTS idx_mov_remesa;
ALTER TABLE movimiento_bancario DROP CONSTRAINT IF EXISTS movimiento_pago_unico_check;
ALTER TABLE movimiento_bancario DROP COLUMN IF EXISTS remesa_id;
