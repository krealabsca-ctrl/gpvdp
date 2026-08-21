ALTER TABLE nomina_archivo_pago DROP CONSTRAINT IF EXISTS uniq_archivo_corrida;
ALTER TABLE finiquito DROP COLUMN IF EXISTS renta;
ALTER TABLE finiquito DROP COLUMN IF EXISTS ccss_obrero;
ALTER TABLE finiquito DROP COLUMN IF EXISTS base_ccss;
