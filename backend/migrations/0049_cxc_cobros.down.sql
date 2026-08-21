DROP TABLE IF EXISTS nota_credito_cxc;
DROP TABLE IF EXISTS cobro_aplicacion;
DROP TABLE IF EXISTS cobro_cxc;
DROP TABLE IF EXISTS cxc_planilla;
DELETE FROM cxc_parametro WHERE clave = 'FECHA_COBRO_DEL_MES';
