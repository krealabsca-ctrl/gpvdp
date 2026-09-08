-- Se borran primero los subpresupuestos: sin la columna no habría cómo distinguirlos del total, y
-- dejarlos convertiría cada uno en un «total» duplicado del departamento.
DELETE FROM presupuesto_departamento WHERE clasificacion_id IS NOT NULL;

DROP INDEX IF EXISTS idx_presupuesto_clasificacion;
DROP INDEX IF EXISTS uq_presupuesto_partida;
DROP INDEX IF EXISTS uq_presupuesto_total;

ALTER TABLE presupuesto_departamento DROP COLUMN IF EXISTS clasificacion_id;

ALTER TABLE presupuesto_departamento
    ADD CONSTRAINT presupuesto_departamento_empresa_id_departamento_id_periodo_key
    UNIQUE (empresa_id, departamento_id, periodo);
