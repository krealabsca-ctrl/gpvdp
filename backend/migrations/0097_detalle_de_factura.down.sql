DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'cxp.gasto');
DELETE FROM permiso WHERE codigo = 'cxp.gasto';

ALTER TABLE documento_cxp DROP COLUMN IF EXISTS detalle_lineas;

DROP TABLE IF EXISTS cxp_linea;
