-- CxP Fase 2+: clasificación de gasto en la factura (reusa el catálogo Concepto/Clasificación
-- del módulo Bancos) + fecha de vencimiento como campo propio para el archivo de pagos maestro
-- (calendarización por vencimiento). El vencimiento antes vivía dentro de `descripcion`.

ALTER TABLE documento_cxp
    ADD COLUMN concepto_id       uuid REFERENCES concepto(id),
    ADD COLUMN clasificacion_id  uuid REFERENCES clasificacion(id),
    ADD COLUMN fecha_vencimiento date;

CREATE INDEX idx_docxp_vencimiento ON documento_cxp (empresa_id, fecha_vencimiento);
CREATE INDEX idx_docxp_concepto ON documento_cxp (concepto_id);
