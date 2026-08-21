DROP TABLE IF EXISTS departamento_validador;
ALTER TABLE documento_cxp DROP CONSTRAINT documento_cxp_estado_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_estado_check
    CHECK (estado IN ('RECIBIDO', 'REVISADO', 'APROBADO', 'PROGRAMADO', 'PAGADO', 'CONCILIADO',
                      'DENEGADO', 'ANULADO', 'LIQUIDADA', 'REBOTADA'));
ALTER TABLE documento_cxp DROP COLUMN IF EXISTS validacion_nota;
ALTER TABLE documento_cxp DROP COLUMN IF EXISTS validacion_respaldo;
ALTER TABLE documento_cxp DROP COLUMN IF EXISTS validado_depto_en;
ALTER TABLE documento_cxp DROP COLUMN IF EXISTS validado_depto_por;
ALTER TABLE documento_cxp DROP COLUMN IF EXISTS departamento_id;
