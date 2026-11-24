-- Revierte la 0109.
--
-- Se pierde el plan de cuentas cargado. Nada se apoya en él todavía —esta es la Fase 1 y ningún
-- módulo lo lee—, así que la reversa es limpia y no cambia ninguna cifra del sistema.
--
-- El día que existan asientos esto ya no será cierto: una cuenta con movimientos registrados no
-- se puede borrar sin borrar la contabilidad. Por eso la migración que cree los asientos tendrá
-- que llevar su propia llave foránea, y esta reversa dejará de ser posible por sí sola.
DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'conta.plan_cuentas');
DELETE FROM permiso WHERE codigo = 'conta.plan_cuentas';

DROP TRIGGER IF EXISTS trg_cuenta_sin_hijas ON cuenta_contable;
DROP FUNCTION IF EXISTS fn_cuenta_sin_hijas();
DROP TABLE IF EXISTS cuenta_contable;
