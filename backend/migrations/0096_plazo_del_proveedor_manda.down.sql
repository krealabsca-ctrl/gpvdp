-- Revierte la elección de origen del plazo.
--
-- Las fechas de vencimiento YA ESCRITAS no se tocan: son un dato del documento, no una vista de
-- esta configuración. Al volver atrás, las facturas nuevas vuelven a tomar el plazo del XML, que
-- es el comportamiento de siempre.

BEGIN;

ALTER TABLE proveedor DROP COLUMN IF EXISTS plazo_manda;

COMMIT;
