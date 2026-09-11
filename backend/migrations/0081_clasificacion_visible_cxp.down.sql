-- Revierte la visibilidad por clasificación.
--
-- Al volver atrás, la visibilidad vuelve a decidirla solo el concepto: **lo que se hubiera ocultado
-- a nivel de clasificación vuelve a verse en CxP**. Es una reapertura de acceso, no una pérdida de
-- datos, y por eso conviene saberlo antes de correr esto.
DROP INDEX IF EXISTS idx_clasificacion_visible_cxp;

ALTER TABLE clasificacion DROP COLUMN IF EXISTS visible_cxp;
