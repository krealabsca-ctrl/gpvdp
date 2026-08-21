ALTER TABLE documento_cxp DROP CONSTRAINT documento_cxp_estado_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_estado_check
    CHECK (estado IN ('RECIBIDO', 'REVISADO', 'APROBADO', 'PROGRAMADO', 'PAGADO', 'CONCILIADO',
                      'DENEGADO', 'ANULADO', 'LIQUIDADA'));

DROP INDEX IF EXISTS idx_docxp_lote;
ALTER TABLE documento_cxp DROP COLUMN IF EXISTS lote_id;
DROP TABLE IF EXISTS lote_pago;
