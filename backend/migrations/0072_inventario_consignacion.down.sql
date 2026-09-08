DROP INDEX IF EXISTS uq_inv_unidad_cxp_consignacion;
ALTER TABLE inv_unidad DROP COLUMN IF EXISTS cxp_consignacion_id;

DROP INDEX IF EXISTS idx_inv_unidad_consignada;

COMMENT ON COLUMN inv_unidad.es_consignada IS NULL;
COMMENT ON COLUMN inv_movimiento.proveedor_id IS NULL;
