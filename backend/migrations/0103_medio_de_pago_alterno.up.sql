-- 0102 · Cerrar una factura que se pagó por fuera del banco, diciendo con qué.
--
-- ── EL PROBLEMA MEDIDO (Director Financiero, 16-oct-2026) ───────────────────────────────────
--
-- «Hay facturas que ya se pagaron pero tampoco las deja liberarlas, y algunas sí se pagaron por
-- caja chica pero solo deja anularlas cuando debería de haber un botón de Caja chica o viáticos.»
--
-- Tenía razón, y la causa es una asimetría entre dos transiciones:
--
--     Anular    acepta  RECIBIDO · REVISADO · VALIDADO_DEPTO · APROBADO · PROGRAMADO
--     Liquidar  acepta  RECIBIDO · REVISADO
--
-- En la pestaña «Por pagar» —que es donde aparece el candado del contado y donde uno descubre el
-- problema— la factura ya está APROBADA o PROGRAMADA. Ahí `Liquidar` no alcanza, así que la única
-- salida ofrecida es ANULAR.
--
-- Y anular una factura que SÍ se pagó es falsear el registro: dice «esta factura no existió»
-- cuando el dinero salió de verdad, por caja chica, tarjeta o efectivo. El gasto desaparece de los
-- libros, el proveedor queda sin su cuenta saldada, y nada permite reconstruir qué pasó.
--
-- ── POR QUÉ LIQUIDADA Y NO PAGADO ───────────────────────────────────────────────────────────
--
-- Decisión del Director Financiero: estas facturas quedan en LIQUIDADA con el medio registrado.
--
-- No es un detalle de nomenclatura. Si quedaran en PAGADO entrarían en la partida conciliatoria
-- que el tablero mide desde hoy —lo declarado como pagado que el banco todavía no confirmó— y
-- NUNCA conciliarían, porque no hay débito con huella que las respalde: no salieron por el banco.
-- Inflarían esa cifra para siempre y la volverían inútil, que es justo el control que acabamos de
-- construir.
--
-- LIQUIDADA ya significaba exactamente esto desde la 0031 («viáticos/almuerzos ya pagados: se
-- archivan sin pago»). Lo que faltaba no era el estado: era poder decir CON QUÉ se pagó.
--
-- ── POR QUÉ EL MOTIVO ES OBLIGATORIO ────────────────────────────────────────────────────────
--
-- Mismo argumento que `liberar` (ver internal/cxp/liberar.go): es una afirmación con
-- consecuencias —«esta factura ya se pagó, por este medio»— y es la ÚNICA barrera que queda
-- contra el doble pago, porque el vale de caja chica no lleva la clave del comprobante. Si en seis
-- meses aparece un pago duplicado, lo único que permite reconstruir qué pasó es quién lo dijo,
-- cuándo y con qué argumento.
--
-- ── EL VALE ES OPCIONAL, A PROPÓSITO ────────────────────────────────────────────────────────
--
-- Decisión del Director Financiero: se liga al vale del fondo si existe, y si no, basta el medio
-- más el motivo. Exigirlo dejaría la factura trancada cuando nadie cargó el vale, que es el mismo
-- problema que esta migración viene a resolver.

ALTER TABLE documento_cxp
    -- Con qué se pagó. NULL = no se cerró por esta vía (el caso de todas las existentes).
    ADD COLUMN medio_pago         text,
    ADD COLUMN medio_pago_motivo  text,
    -- El vale del fondo fijo, cuando existe. Solo tiene sentido con CAJA_CHICA (ver el CHECK).
    ADD COLUMN medio_pago_vale_id uuid REFERENCES caja_chica_vale (id),
    ADD COLUMN medio_pago_en      timestamptz,
    ADD COLUMN medio_pago_por     uuid REFERENCES usuario (id);

-- Los cuatro medios que la operación usa hoy. Un medio nuevo entra por migración y no por una
-- cadena suelta: una columna de texto libre acá se llena de «cajachica», «Caja Chica» y «CC», y
-- entonces ningún reporte puede agrupar por medio.
ALTER TABLE documento_cxp
    ADD CONSTRAINT documento_medio_pago_check
        CHECK (medio_pago IS NULL
               OR medio_pago IN ('CAJA_CHICA', 'VIATICOS', 'TARJETA', 'EFECTIVO'));

-- El vale pertenece al fondo fijo: ligarlo a un pago con tarjeta diría que salió de la caja.
ALTER TABLE documento_cxp
    ADD CONSTRAINT documento_vale_solo_caja_chica_check
        CHECK (medio_pago_vale_id IS NULL OR medio_pago = 'CAJA_CHICA');

-- Un medio declarado sin motivo deja el expediente sin nada que leer, que es la mitad del valor.
ALTER TABLE documento_cxp
    ADD CONSTRAINT documento_medio_pago_con_motivo_check
        CHECK (medio_pago IS NULL OR COALESCE(btrim(medio_pago_motivo), '') <> '');

-- Para el reporte «qué se cerró sin pasar por el banco», que es la contraparte de la partida
-- conciliatoria: parcial porque la enorme mayoría de las filas tiene la columna en NULL.
CREATE INDEX idx_doc_medio_pago ON documento_cxp (empresa_id, medio_pago)
    WHERE medio_pago IS NOT NULL;
