BEGIN;

DELETE FROM rol_permiso
WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo IN ('cxp.aprobar_contabilidad', 'cxp.marcar_contabilidad'));

DELETE FROM permiso WHERE codigo IN ('cxp.aprobar_contabilidad', 'cxp.marcar_contabilidad');

DROP INDEX IF EXISTS idx_docxp_contabilidad;

ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS contabilidad_marcado_en,
    DROP COLUMN IF EXISTS contabilidad_marcado_por,
    DROP COLUMN IF EXISTS contabilidad_motivo,
    DROP COLUMN IF EXISTS es_contabilidad;

ALTER TABLE clasificacion DROP COLUMN IF EXISTS es_contabilidad;
ALTER TABLE concepto DROP COLUMN IF EXISTS es_contabilidad;
ALTER TABLE proveedor DROP COLUMN IF EXISTS es_contabilidad;

COMMIT;
