BEGIN;

DROP INDEX IF EXISTS idx_concepto_naturaleza;
ALTER TABLE concepto DROP CONSTRAINT IF EXISTS concepto_naturaleza_check;
ALTER TABLE concepto DROP COLUMN IF EXISTS naturaleza;

COMMIT;
