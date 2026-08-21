DROP TABLE IF EXISTS arreglo_cuota_cxc;
DROP TABLE IF EXISTS arreglo_pago_cxc;

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'cxc.preventivo');
DELETE FROM permiso WHERE codigo = 'cxc.preventivo';

DELETE FROM cxc_parametro
WHERE clave IN ('ARREGLO_PLAZOS_ESTANDAR', 'ARREGLO_PLAZO_MAXIMO', 'DIAS_CONTACTO_PREVENTIVO');
