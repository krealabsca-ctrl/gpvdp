DROP TABLE IF EXISTS acta_conciliacion;
DROP TABLE IF EXISTS partida_conciliacion;
ALTER TABLE saldo_cuenta_diario DROP COLUMN IF EXISTS revisado_en;
ALTER TABLE saldo_cuenta_diario DROP COLUMN IF EXISTS revisado_por;
