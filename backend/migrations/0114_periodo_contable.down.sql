-- Revierte la 0113.

DROP TRIGGER IF EXISTS trg_linea_periodo_abierto ON asiento_linea;
DROP TRIGGER IF EXISTS trg_asiento_periodo_abierto ON asiento;
DROP FUNCTION IF EXISTS fn_periodo_no_cerrado();

DROP TABLE IF EXISTS periodo_contable;

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'conta.periodos');
DELETE FROM permiso WHERE codigo = 'conta.periodos';
