DROP TABLE IF EXISTS promesa_pago_cxc;
DROP TABLE IF EXISTS gestion_cxc;
DROP TABLE IF EXISTS cxc_resultado_gestion;
DROP TABLE IF EXISTS cxc_canal_gestion;
DELETE FROM cxc_parametro WHERE clave IN ('DIAS_SIN_GESTIONAR', 'PROMESA_TOLERANCIA_DIAS');
