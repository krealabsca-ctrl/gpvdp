-- 0098 · Hasta dónde está al día cada contrato.
--
-- EL PROBLEMA, medido (13-oct-2026). Los cargos se generan para un rango —enero a octubre de
-- 2026— y se le aplicaban COMPLETOS a todos los contratos activos, sin mirar hasta cuándo había
-- pagado cada cliente. Resultado contra el saldo que traía el propio archivo de cartera:
--
--     18.450 contratos con saldo de MÁS, por ₡1.042.683.725
--
-- El caso que lo destapó: CO118040 debe UNA cuota de ₡500.000 según el archivo, y el sistema le
-- calculó NUEVE: ₡4.500.000. Lo mismo CO179887 (₡395.500 contra ₡3.559.500) y CO139317
-- (₡480.000 contra ₡2.160.000).
--
-- EL DATO YA VENÍA EN EL ARCHIVO y el lector ya lo reconocía: «Fecha próximo pago». Pero solo se
-- usaba como respaldo cuando faltaba la de primer cobro, y no se guardaba en ninguna parte. Es la
-- fecha de la próxima cuota que al cliente le toca pagar, o sea el piso de su deuda: nada
-- anterior a esa fecha se le debe cobrar.
--
-- NO MUEVE NI UN CARGO. Agrega la columna, nada más. Los cargos ya generados siguen como están;
-- corregirlos es una operación aparte, que se hace a propósito y mirando el resultado.
ALTER TABLE contrato_cxc
    ADD COLUMN IF NOT EXISTS fecha_proximo_pago date;

COMMENT ON COLUMN contrato_cxc.fecha_proximo_pago IS
  'La próxima cuota que al cliente le toca pagar, según el archivo de cartera. Es el PISO de su '
  'deuda: la generación de cargos no crea nada anterior a esta fecha. NULL = el archivo no la '
  'traía, y entonces manda la fecha de primer cobro, como antes.';

-- La generación recorre los contratos activos de una empresa y mira esta fecha en cada uno.
CREATE INDEX IF NOT EXISTS idx_contrato_cxc_proximo_pago
    ON contrato_cxc (empresa_id, fecha_proximo_pago)
    WHERE fecha_proximo_pago IS NOT NULL;
