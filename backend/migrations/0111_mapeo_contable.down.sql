-- Revierte la 0110.
--
-- Se pierde el mapeo: qué cuenta corresponde a cada partida, y los motivos de las que se
-- declararon no contabilizables. Eso es criterio contable escrito a mano, no un dato derivable,
-- así que revertir cuesta rehacer el trabajo.
--
-- Nada más se rompe: en la Fase 2 el mapeo todavía no genera asientos, y el plan de cuentas y las
-- clasificaciones quedan intactos. El día que existan asientos, esta reversa dejará de ser
-- inocua y la migración que los cree tendrá que decirlo.
DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'conta.mapeo');
DELETE FROM permiso WHERE codigo = 'conta.mapeo';

DROP TRIGGER IF EXISTS trg_mapeo_cuenta_de_movimiento ON mapeo_contable;
DROP FUNCTION IF EXISTS fn_mapeo_cuenta_de_movimiento();
DROP TABLE IF EXISTS mapeo_contable;

COMMENT ON COLUMN clasificacion.cuenta_contable_futura IS NULL;
