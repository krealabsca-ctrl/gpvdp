ALTER TABLE documento_cxp DROP COLUMN IF EXISTS clasif_auto;
ALTER TABLE proveedor
    DROP COLUMN IF EXISTS gasto_subclasificacion_id,
    DROP COLUMN IF EXISTS gasto_clasificacion_id,
    DROP COLUMN IF EXISTS gasto_concepto_id;
