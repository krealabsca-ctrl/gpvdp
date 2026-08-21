-- CxP: tipo de factura + estados terminales del ciclo de revisión.
-- Tipo: CXP (normal), ANTICIPO, VIATICOS (gasto ya liquidado, no se paga), REINTEGRO (caja chica).
-- Estados terminales nuevos: DENEGADO, ANULADO (fuera del flujo de pago), LIQUIDADA (viáticos archivados sin pago).

ALTER TABLE documento_cxp
    ADD COLUMN tipo text NOT NULL DEFAULT 'CXP'
        CHECK (tipo IN ('CXP', 'ANTICIPO', 'VIATICOS', 'REINTEGRO'));

ALTER TABLE documento_cxp DROP CONSTRAINT documento_cxp_estado_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_estado_check
    CHECK (estado IN ('RECIBIDO', 'REVISADO', 'APROBADO', 'PROGRAMADO', 'PAGADO', 'CONCILIADO',
                      'DENEGADO', 'ANULADO', 'LIQUIDADA'));

CREATE INDEX idx_docxp_tipo ON documento_cxp (empresa_id, tipo);
