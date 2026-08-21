-- Bandeja CxP: memoria de gasto por proveedor (auto-clasificación) + marca AUTO en el documento.
-- Al clasificar una factura, la categoría queda como predeterminada del proveedor; las siguientes
-- facturas de ese proveedor nacen pre-clasificadas (clasif_auto = true) y solo se confirman.

ALTER TABLE proveedor
    ADD COLUMN gasto_concepto_id        uuid REFERENCES concepto(id),
    ADD COLUMN gasto_clasificacion_id   uuid REFERENCES clasificacion(id),
    ADD COLUMN gasto_subclasificacion_id uuid REFERENCES subclasificacion(id);

ALTER TABLE documento_cxp ADD COLUMN clasif_auto boolean NOT NULL DEFAULT false;
