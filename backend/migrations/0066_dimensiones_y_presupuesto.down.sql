DROP TABLE IF EXISTS presupuesto_departamento;

DROP INDEX IF EXISTS idx_mov_sede;
DROP INDEX IF EXISTS idx_mov_departamento;

ALTER TABLE movimiento_bancario
    DROP COLUMN IF EXISTS sede_id,
    DROP COLUMN IF EXISTS departamento_id;

ALTER TABLE clasificacion
    DROP COLUMN IF EXISTS sede_id,
    DROP COLUMN IF EXISTS departamento_id;

DROP INDEX IF EXISTS idx_sede_empresa;
DROP TABLE IF EXISTS sede;
