-- RRHH / Nómina — Etapa 3, endurecimiento tras revisión adversarial.
--
-- 1) El finiquito retiene CCSS y renta sobre su porción AFECTA (las vacaciones pendientes
--    SÍ son salario; la cesantía y el aguinaldo son exentos). Antes se pagaban en bruto.
-- 2) El archivo de pago pasa a ser IDEMPOTENTE por corrida: un consecutivo por corrida
--    (regenerar la descarga no quema consecutivos ni rompe la trazabilidad de la bitácora).

ALTER TABLE finiquito ADD COLUMN base_ccss   numeric(14, 2) NOT NULL DEFAULT 0;
ALTER TABLE finiquito ADD COLUMN ccss_obrero numeric(14, 2) NOT NULL DEFAULT 0;
ALTER TABLE finiquito ADD COLUMN renta       numeric(14, 2) NOT NULL DEFAULT 0;

-- Un solo consecutivo por corrida: la bitácora registra el archivo, no cada descarga.
DELETE FROM nomina_archivo_pago a
WHERE EXISTS (
    SELECT 1 FROM nomina_archivo_pago b
    WHERE b.empresa_id = a.empresa_id AND b.corrida_id = a.corrida_id
      AND (b.consecutivo < a.consecutivo)
);
ALTER TABLE nomina_archivo_pago ADD CONSTRAINT uniq_archivo_corrida UNIQUE (empresa_id, corrida_id);
