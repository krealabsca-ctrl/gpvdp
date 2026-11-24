-- El cronograma de pago: lo que se manda ANTES de pagar (11-nov-2026).
--
-- «Los cronogramas de pago se envían por correo con lo que se pretende pagar y los bancos de donde
-- salen los fondos.»
--
-- El Excel que esto reemplaza arma una cascada de cuatro restas, y sus cifras cierran al céntimo
-- (verificado sobre el archivo real):
--
--     Colones + Dólares colonizados   =  52.110.734,73   «TOTAL COLONIZADO»
--     − Reserva BP Negocios           =  51.888.184,73   «TOTAL FLUJO NORMAL VALLE»
--     − Solicitudes por fuera         =  10.932.828,47   «CRONOGRAMA TOTAL FLUJO NORMAL»
--     − Dólares                       =  10.604.337,47   «CRONOGRAMA COLONES FLUJO NORMAL»
--
-- Para producir eso el sistema necesitaba TRES datos que no guardaba, más el tipo de cambio.
--
-- 1) DE QUÉ CUENTA SALE CADA PAGO (`documento_cxp.cuenta_pago_id`). Sin esto no se puede separar
--    la reserva del flujo normal, que es la primera resta de la cascada. Va en la FACTURA y no en
--    el corte porque un mismo corte reparte sus pagos entre varias cuentas: es justamente lo que
--    el cuadro muestra.
--
-- 2) QUÉ CUENTA ES RESERVA (`cuenta_bancaria.es_reserva`). Decisión del Director Financiero: la
--    reserva se define por LA CUENTA DE LA QUE SALE, no por el proveedor ni por el rubro. Así
--    «TOTAL RESERVA BP NEGOCIOS» se calcula solo y deja de escribirse a mano.
--
-- 3) QUÉ SE PAGA POR FUERA DEL CRONOGRAMA (`documento_cxp.por_fuera`). Decisión del Director
--    Financiero: se marca AL ARMAR EL CORTE. Son pagos que salen igual pero no entran en el
--    cronograma que se comunica —en el archivo real son ₡40,9 M de ₡51,9 M, así que sin esta
--    marca el cronograma diría casi cinco veces lo que de verdad comunica—.
--
-- 4) EL TIPO DE CAMBIO DEL CRONOGRAMA (`lote_pago.tc_cronograma`). Decisión del Director
--    Financiero: LO ESCRIBE TESORERÍA. No se usa el TC congelado del mes a propósito: el del
--    archivo real es 450,0000 exacto, un número elegido por quien arma el cuadro para proyectar,
--    no la cotización del día. Queda guardado con el corte para que reimprimir el cronograma de
--    hace un mes dé las mismas cifras que dio entonces.

ALTER TABLE cuenta_bancaria
    ADD COLUMN es_reserva boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN cuenta_bancaria.es_reserva IS
    'La cuenta guarda fondos reservados: lo que sale de acá se resta del flujo normal en el cronograma. Lo define la CUENTA, no el proveedor ni el rubro.';

ALTER TABLE documento_cxp
    -- De qué cuenta sale. NULL = todavía no se decidió, y el cronograma lo cuenta como flujo
    -- normal: es lo que pasa con todo lo cargado antes de esta migración, y tratarlo como reserva
    -- habría sacado del cronograma plata que sí se comunica.
    ADD COLUMN cuenta_pago_id uuid REFERENCES cuenta_bancaria(id),
    ADD COLUMN por_fuera boolean NOT NULL DEFAULT false,
    -- Quién la marcó y cuándo: sacar un pago del cronograma es una decisión, no un atributo de la
    -- factura, y sin autor no se puede preguntar por qué ₡40 M no se comunicaron.
    ADD COLUMN por_fuera_por uuid REFERENCES usuario(id),
    ADD COLUMN por_fuera_en timestamptz;

CREATE INDEX idx_docxp_cuenta_pago ON documento_cxp (cuenta_pago_id)
    WHERE cuenta_pago_id IS NOT NULL;
-- El cronograma filtra por esto en cada corte; son pocas filas y conviene que estén juntas.
CREATE INDEX idx_docxp_por_fuera ON documento_cxp (empresa_id, lote_id) WHERE por_fuera;

ALTER TABLE lote_pago
    ADD COLUMN tc_cronograma numeric(16, 6);

COMMENT ON COLUMN lote_pago.tc_cronograma IS
    'El tipo de cambio con el que se colonizaron los dólares de ESTE cronograma, escrito por tesorería. Se guarda con el corte para que reimprimirlo dé las mismas cifras; no es el TC congelado del mes.';

ALTER TABLE lote_pago
    ADD CONSTRAINT ck_lote_tc_cronograma_positivo
        CHECK (tc_cronograma IS NULL OR tc_cronograma > 0);
