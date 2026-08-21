DROP INDEX IF EXISTS idx_docxp_tipo;

ALTER TABLE documento_cxp DROP CONSTRAINT documento_cxp_estado_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_estado_check
    CHECK (estado IN ('RECIBIDO', 'REVISADO', 'APROBADO', 'PROGRAMADO', 'PAGADO', 'CONCILIADO'));

ALTER TABLE documento_cxp DROP COLUMN IF EXISTS tipo;
