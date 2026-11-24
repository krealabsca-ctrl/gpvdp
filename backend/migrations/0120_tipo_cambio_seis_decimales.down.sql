-- Volver el tipo de cambio a 4 decimales.
--
-- ESTO SÍ PIERDE DATOS, al revés que la subida: un TC guardado como 456,673333 vuelve a ser
-- 456,6733 y no hay forma de recuperar los dos decimales. Los montos ya calculados en colones no
-- cambian —están guardados aparte— así que después de bajar quedarían montos que su propio TC ya
-- no explica.
--
-- `asiento_linea.tc_aplicado` no se toca: siempre fue numeric(18,6), desde antes de esta migración.

ALTER TABLE documento_cxp
    ALTER COLUMN tc_aplicado TYPE numeric(14, 4);

ALTER TABLE movimiento_bancario
    ALTER COLUMN tc_aplicado TYPE numeric(14, 4);

ALTER TABLE tipo_cambio_mes
    ALTER COLUMN valor_congelado TYPE numeric(14, 4);

ALTER TABLE tipo_cambio_cotizacion
    ALTER COLUMN valor TYPE numeric(14, 4);
