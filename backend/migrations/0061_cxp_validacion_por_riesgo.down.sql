BEGIN;

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'cxp.parametros');
DELETE FROM permiso WHERE codigo = 'cxp.parametros';

DROP INDEX IF EXISTS idx_docxp_requiere_validacion;
ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS validacion_motivo,
    DROP COLUMN IF EXISTS requiere_validacion;

DROP TABLE IF EXISTS cxp_parametro;

COMMIT;
