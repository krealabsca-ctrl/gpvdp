DROP TABLE IF EXISTS cxc_planilla_movimiento;

DELETE FROM cxc_parametro WHERE clave = 'PLANILLA_TOLERANCIA';

ALTER TABLE cxc_planilla DROP CONSTRAINT IF EXISTS cxc_planilla_periodo_no_vacio;
ALTER TABLE cxc_planilla DROP CONSTRAINT IF EXISTS cxc_planilla_periodo_key;
ALTER TABLE cxc_planilla
    ADD COLUMN esperado numeric(16, 2) NOT NULL DEFAULT 0,
    ADD COLUMN depositado numeric(16, 2) NOT NULL DEFAULT 0,
    ADD COLUMN fechas_bancarias date[] NOT NULL DEFAULT '{}',
    ADD COLUMN estado text NOT NULL DEFAULT 'PENDIENTE'
        CHECK (estado IN ('PENDIENTE', 'RECIBIDA', 'CONCILIADA', 'CON_DIFERENCIA'));
ALTER TABLE cxc_planilla ADD CONSTRAINT cxc_planilla_empresa_id_asociacion_id_referencia_key
    UNIQUE (empresa_id, asociacion_id, referencia);

DROP INDEX IF EXISTS idx_cxc_planilla_empresa;
CREATE INDEX idx_cxc_planilla_empresa ON cxc_planilla (empresa_id, estado);
