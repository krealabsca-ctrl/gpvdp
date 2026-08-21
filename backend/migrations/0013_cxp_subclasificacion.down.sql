DROP INDEX IF EXISTS idx_docxp_subclasif;
ALTER TABLE documento_cxp DROP COLUMN IF EXISTS subclasificacion_id;
DROP TABLE IF EXISTS subclasificacion;
