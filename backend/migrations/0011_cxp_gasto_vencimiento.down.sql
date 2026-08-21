DROP INDEX IF EXISTS idx_docxp_concepto;
DROP INDEX IF EXISTS idx_docxp_vencimiento;
ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS fecha_vencimiento,
    DROP COLUMN IF EXISTS clasificacion_id,
    DROP COLUMN IF EXISTS concepto_id;
