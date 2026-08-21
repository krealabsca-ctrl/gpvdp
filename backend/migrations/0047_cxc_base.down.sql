-- Reversa de la fase 1 de CxC. Orden inverso por las dependencias.
DROP TABLE IF EXISTS cxc_importacion;
DROP TABLE IF EXISTS cxc_parametro;
DROP TABLE IF EXISTS cargo_cxc;
DROP TABLE IF EXISTS contrato_cxc;
DROP TABLE IF EXISTS cxc_tramo;
DROP TABLE IF EXISTS cxc_forma_pago;
DROP TABLE IF EXISTS cxc_modalidad;
DROP TABLE IF EXISTS cxc_asociacion;
DROP TABLE IF EXISTS cxc_sede;
-- pg_trgm no se elimina: lo puede estar usando otro módulo.
