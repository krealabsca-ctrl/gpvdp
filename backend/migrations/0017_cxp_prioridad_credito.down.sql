DROP TABLE IF EXISTS proveedor_gasto;
ALTER TABLE proveedor
    DROP COLUMN IF EXISTS plazo_credito_dias,
    DROP COLUMN IF EXISTS condicion_pago;
ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS nota_revision,
    DROP COLUMN IF EXISTS prioridad;
