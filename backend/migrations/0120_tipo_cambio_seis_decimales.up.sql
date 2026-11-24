-- El tipo de cambio pasa de 4 a 6 decimales (10-nov-2026).
--
-- «Debería permitir hasta 6 decimales, para que la ecuación nos quede bien; aunque del BCCR usa dos
-- decimales, al usar el promedio debe permitir hasta 6 decimales si el promedio lo tiene.»
--
-- El TC del mes es el PROMEDIO de tres cotizaciones (día 1, 15 y último), y un promedio de tres
-- casi nunca es exacto: (460,73 + 453,25 + 456,04) / 3 = 456,673333… que con cuatro decimales se
-- guardaba como 456,6733. Esa diferencia multiplica cada monto en dólares del mes, y es justo la
-- que impide que la conversión cuadre contra el estado de cuenta.
--
-- SE AMPLÍAN LAS CUATRO COLUMNAS, NO SOLO LAS DOS DEL TIPO DE CAMBIO. El valor congelado viaja al
-- movimiento (`movimiento_bancario.tc_aplicado`) y a la factura (`documento_cxp.tc_aplicado`): con
-- esas dos en cuatro decimales, el promedio se volvía a redondear al aplicarse y ampliar el origen
-- no habría servido de nada. La contabilidad ya guardaba seis (`asiento_linea.tc_aplicado`,
-- numeric(18,6)), así que esto además deja de ser inconsistente consigo mismo.
--
-- NO CAMBIA NINGÚN VALOR YA GUARDADO: 452,6000 pasa a ser 452,600000, que es el mismo número. Los
-- meses ya congelados se quedan como están —decisión del Director Financiero— porque sus cifras ya
-- se reportaron y sus movimientos se calcularon con ellas. Los próximos congelamientos salen con
-- seis decimales.
--
-- OJO AL APLICARLA EN PRODUCCIÓN: cambiar la escala de un numeric obliga a PostgreSQL a reescribir
-- la tabla entera y a tomar un candado sobre ella. Con ~200 000 movimientos son segundos, pero son
-- segundos en los que esa tabla no se puede leer ni escribir: conviene correrla fuera de horario.

ALTER TABLE tipo_cambio_cotizacion
    ALTER COLUMN valor TYPE numeric(16, 6);

ALTER TABLE tipo_cambio_mes
    ALTER COLUMN valor_congelado TYPE numeric(16, 6);

ALTER TABLE movimiento_bancario
    ALTER COLUMN tc_aplicado TYPE numeric(16, 6);

ALTER TABLE documento_cxp
    ALTER COLUMN tc_aplicado TYPE numeric(16, 6);

COMMENT ON COLUMN tipo_cambio_mes.valor_congelado IS
    'TC del mes = promedio(día 1, 15, último) con 6 decimales. El promedio de tres rara vez es exacto y los decimales que se pierdan multiplican cada monto en dólares del mes.';
