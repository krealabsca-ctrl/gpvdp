DELETE FROM rol_permiso WHERE rol_id IN (SELECT id FROM rol WHERE codigo = 'SUPERVISOR_PISO');
DELETE FROM usuario_empresa_rol WHERE rol_id IN (SELECT id FROM rol WHERE codigo = 'SUPERVISOR_PISO');
DELETE FROM rol WHERE codigo = 'SUPERVISOR_PISO';

DROP TABLE IF EXISTS nota_credito_aplicacion;
DROP INDEX IF EXISTS idx_nota_credito_cxc_empresa;

ALTER TABLE nota_credito_cxc DROP CONSTRAINT IF EXISTS nota_credito_consecutivo_key;
ALTER TABLE nota_credito_cxc
    DROP COLUMN IF EXISTS consecutivo,
    DROP COLUMN IF EXISTS anulada_por,
    DROP COLUMN IF EXISTS anulada_en,
    DROP COLUMN IF EXISTS anulacion_motivo;
