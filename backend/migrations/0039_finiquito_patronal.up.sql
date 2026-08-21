-- RRHH / Nómina — Etapa 5: el finiquito entra a la planilla CCSS del mes.
--
-- Las vacaciones pendientes que se pagan al cese SON salario: cotizan obrero (ya se
-- retenía desde 0035) y también generan CARGA PATRONAL. Se guarda en el snapshot del
-- finiquito, igual que en la colilla, para que la planilla del mes reporte el costo
-- congelado al momento de aprobar y no dependa de los parámetros vigentes hoy.

ALTER TABLE finiquito ADD COLUMN patronal numeric(14, 2) NOT NULL DEFAULT 0;
