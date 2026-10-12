-- Los pagos que YA se aplicaron en el sistema anterior, para que el ERP deje de contarlos como
-- pendientes sin dejar de mostrarlos.
--
-- EL PROBLEMA QUE RESUELVE
--
-- Valle de Paz tiene 3.437 débitos sin enlazar por ₡1.831.621.463. El ERP no puede distinguir
-- entre «este pago nunca se registró» y «este pago ya está aplicado en el otro sistema», así que
-- los trata a todos como pendientes y la cifra asusta sin significar nada. Qué se aplicó allá no se
-- deduce de ningún dato de acá: solo lo sabe quien tiene el reporte del otro ERP.
--
-- POR QUÉ UNA TABLA Y NO UNAS COLUMNAS EN `movimiento_bancario`
--
--   · Es un hecho de OTRO sistema, no un atributo del movimiento. Mezclarlos haría que el día que
--     se apague el sistema viejo no se sepa qué limpiar.
--   · Guarda el renglón del reporte como venía (referencia, documento, tercero, monto reportado).
--     Sin eso, un cruce mal hecho no se puede auditar después: quedaría la marca y no la prueba.
--   · El monto reportado se guarda SIN exigir que coincida con el del banco. Cuando difieren eso es
--     un hallazgo —no una razón para rechazar la fila—, igual que en la conciliación por huella.
--   · `carga_id` permite revertir una carga entera. Una importación equivocada que marca 2.000
--     movimientos como aplicados tiene que poder deshacerse de una vez.
--
-- DECISIÓN DEL DIRECTOR FINANCIERO (6-oct-2026): el movimiento marcado **sigue visible**, con el
-- sello de «aplicado en el sistema anterior». No se esconde: la plata salió del banco igual, y un
-- movimiento que desaparece de la lista es un movimiento que nadie vuelve a revisar.

CREATE TABLE IF NOT EXISTS pago_aplicado_externo (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id             uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    movimiento_bancario_id uuid NOT NULL REFERENCES movimiento_bancario(id) ON DELETE CASCADE,

    -- Lo que decía el reporte del otro ERP, tal cual. Todo opcional menos el movimiento: un reporte
    -- puede traer referencia y no documento, o al revés, y rechazar la fila por eso sería perder el
    -- dato que sí trae.
    referencia      text NOT NULL DEFAULT '',
    documento       text NOT NULL DEFAULT '',
    tercero         text NOT NULL DEFAULT '',
    fecha_aplicado  date,
    monto_reportado numeric(16,2),
    nota            text NOT NULL DEFAULT '',

    -- Trazabilidad de la carga.
    carga_id    uuid NOT NULL,
    cargado_por uuid REFERENCES usuario(id),
    cargado_en  timestamptz NOT NULL DEFAULT now()
);

-- Un movimiento se aplica UNA vez. Si el reporte lo trae dos veces, la segunda es un duplicado del
-- archivo y no un segundo pago: sin esto, el mismo débito quedaría marcado por dos renglones y
-- cualquier conteo lo contaría doble.
CREATE UNIQUE INDEX IF NOT EXISTS idx_pago_externo_movimiento
    ON pago_aplicado_externo (movimiento_bancario_id);

-- Por empresa y por carga: el listado filtra por la primera y la reversa borra por la segunda.
CREATE INDEX IF NOT EXISTS idx_pago_externo_empresa ON pago_aplicado_externo (empresa_id);
CREATE INDEX IF NOT EXISTS idx_pago_externo_carga ON pago_aplicado_externo (carga_id);

-- La referencia es por donde se busca cuando alguien tiene el número del otro sistema en la mano.
CREATE INDEX IF NOT EXISTS idx_pago_externo_referencia
    ON pago_aplicado_externo (empresa_id, referencia)
    WHERE referencia <> '';

COMMENT ON TABLE pago_aplicado_externo IS
    'Pagos que el sistema anterior ya tenía aplicados. El movimiento sigue visible con el sello; esto NO lo enlaza a una factura del ERP.';
