DROP TABLE IF EXISTS corrida_novedad;
DROP TABLE IF EXISTS corrida_linea;
DROP TABLE IF EXISTS corrida_nomina;
ALTER TABLE nomina_parametros DROP COLUMN IF EXISTS cesantia_pct;
ALTER TABLE nomina_parametros DROP COLUMN IF EXISTS vacaciones_pct;
ALTER TABLE nomina_parametros DROP COLUMN IF EXISTS aguinaldo_pct;
ALTER TABLE empleado DROP COLUMN IF EXISTS conyuge;
ALTER TABLE empleado DROP COLUMN IF EXISTS hijos;
