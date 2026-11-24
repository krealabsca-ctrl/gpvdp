-- 0117 · El aviso de pago: el papel que se manda cuando se paga.
--
-- ── EL PEDIDO ───────────────────────────────────────────────────────────────────────────────
--
-- Textual del Director Financiero (4-nov-2026): «cada vez que se manda a pagar algo se envía un
-- correo con un cuadro así para los que se van a pagar, ¿podés crear el formato en Excel para que
-- salga así?». El cuadro se arma hoy a mano en Excel, corte tras corte: Proveedor · Moneda ·
-- Concepto · Documentos · Monto · Cuenta Bancaria · Identificación.
--
-- ── SEIS DE LAS SIETE COLUMNAS YA EXISTEN ───────────────────────────────────────────────────
--
-- Y existen en el MISMO lugar del que sale el archivo que se sube al banco (`PagoRow`): cédula,
-- nombre, IBAN, moneda y monto neto. Que el aviso se derive de ahí no es comodidad: es lo que
-- hace imposible que el papel que recibe el proveedor diga algo distinto de lo que salió del
-- banco. Dos consultas separadas se desincronizan; ésta no puede.
--
-- Las dos que faltan son TEXTO que hoy alguien escribe:
--
--   · «Concepto»  — «Urnas Personalizadas», «Suministro de Gas». Se propone del catálogo de gasto
--     de las facturas, pero el catálogo dice «Gastos › Mantenimiento» y el cuadro dice algo que el
--     proveedor reconoce. No son lo mismo.
--   · «Documentos» — «FC 07 10 13 14 URNAS ORSM 2572 2760 2822 2881 DANIEL CHAVES». Los
--     consecutivos se proponen solos; la glosa de atrás la escribe quien arma el corte y es la que
--     permite que el proveedor ubique el pago contra sus propias órdenes.
--
-- Decisión del usuario: AUTOMÁTICO Y EDITABLE. El sistema propone y se puede corregir.
--
-- ── POR QUÉ UNA TABLA Y NO DOS COLUMNAS EN `remesa_pago` ────────────────────────────────────
--
-- Porque la remesa solo existe cuando el corte se arma POR PROVEEDOR; en modo FACTURA no hay
-- ninguna fila donde colgar el texto, y el aviso se manda igual. La clave es (corte, proveedor),
-- que es la unidad del cuadro en los dos modos: una fila por proveedor, con todos sus documentos.
--
-- ── POR QUÉ SE GUARDA, EN VEZ DE CALCULARLO AL VUELO ────────────────────────────────────────
--
-- Porque es la prueba de lo que se le dijo al proveedor. Si el texto se recalculara, reimprimir el
-- aviso de un corte de hace seis meses podría dar algo distinto —el catálogo cambió, la factura se
-- reclasificó— y el papel dejaría de respaldar al correo que se mandó ese día.

CREATE TABLE aviso_pago_texto (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id   uuid NOT NULL REFERENCES empresa(id),
    lote_id      uuid NOT NULL REFERENCES lote_pago(id),
    proveedor_id uuid NOT NULL REFERENCES proveedor(id),

    -- NULL significa «no se tocó, usá lo que el sistema propone». Cadena vacía significa «lo
    -- borré a propósito»: son cosas distintas y por eso no se colapsan en un DEFAULT ''.
    concepto   text,
    documentos text,

    actualizado_por uuid REFERENCES usuario(id),
    actualizado_en  timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT ux_aviso_texto_corte_proveedor UNIQUE (empresa_id, lote_id, proveedor_id),
    -- Un texto de 4.000 caracteres no cabe en la celda ni en el correo: el tope frena el pegote
    -- accidental antes de que llegue al papel.
    CONSTRAINT ck_aviso_texto_largo CHECK (
        coalesce(length(concepto), 0) <= 120 AND coalesce(length(documentos), 0) <= 600)
);

COMMENT ON TABLE aviso_pago_texto IS
  'Texto editable del aviso de pago por (corte, proveedor): concepto y glosa de documentos. '
  'NULL = usar lo que el sistema propone; vacío = borrado a propósito.';

CREATE INDEX idx_aviso_texto_lote ON aviso_pago_texto (empresa_id, lote_id);
