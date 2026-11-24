-- 0106 · La nota de crédito del proveedor: el documento que RESTA.
--
-- ── EL PROBLEMA (auditoría Bancos↔CxP, hallazgo 3.6) ────────────────────────────────────────
--
-- El neto a pagar es `total − retención − anticipos − pagos parciales`. No había tabla, estado ni
-- campo para una nota de crédito emitida por el proveedor.
--
-- Es un hueco de dominio con efecto EN CADENA, y la cadena es lo que lo vuelve grave: el proveedor
-- emite una NC → el neto del sistema queda inflado → el banco debita menos → el barrido devuelve
-- MONTO_DIFERENTE → y hasta la migración 0105 eso no tenía salida. Una nota de crédito convertía
-- una factura en algo que no se podía conciliar nunca.
--
-- ── EL SISTEMA YA SABÍA QUE FALTABA ─────────────────────────────────────────────────────────
--
-- El lector de XML de Hacienda reconoce los 7 tipos de comprobante desde que existe, y excluye la
-- nota de crédito con este comentario, textual:
--
--     «la nota de crédito RESTA: entraría sumando»
--
-- O sea: se sabía que leerla como una factura más inflaría la deuda, y se eligió no leerla hasta
-- tener dónde ponerla. Esto es ese «dónde».
--
-- ── POR QUÉ ES UN documento_cxp Y NO UNA TABLA APARTE ───────────────────────────────────────
--
-- CxC resolvió sus notas de crédito con una tabla propia (`nota_credito_cxc`), y acá NO se copia
-- ese molde, porque los dos documentos no son la misma cosa:
--
--   · La NC de CxC la emitimos NOSOTROS: es un ajuste interno sobre un cargo.
--   · La NC de CxP la emite el PROVEEDOR y es un comprobante electrónico de Hacienda, con su
--     clave de 50 dígitos, su consecutivo y su XML.
--
-- `documento_cxp` ya tiene unicidad por `(empresa_id, clave)`, que es el anti-duplicado contra
-- Hacienda —4.542 de 4.542 facturas la traen—, además del enlace al proveedor, el adjunto del
-- comprobante y el listado. Una tabla aparte perdería todo eso, empezando por lo que más importa:
-- que la misma NC no se pueda cargar dos veces.
--
-- ── SE APLICA A FACTURAS ESPECÍFICAS ────────────────────────────────────────────────────────
--
-- Decisión del Director Financiero (20-oct-2026). Una NC corrige una factura concreta —un precio
-- mal facturado, una devolución— y por eso se elige contra cuál se aplica, igual que un anticipo.
-- La alternativa, dejarla como saldo a favor que se consume solo, obligaría a decidir en qué orden
-- se consume, y esa es una regla que nadie pidió.
--
-- NO SE REUSA `anticipo_aplicacion`, aunque la forma sea idéntica. Un anticipo es plata que YA se
-- le entregó al proveedor —exige estado PAGADO— y una NC es una reducción de lo que se le debe,
-- que no se paga nunca. Meterlos en la misma tabla obligaría a que el estado «pagado» signifique
-- dos cosas, y el saldo del anticipo —cuánto queda de lo entregado— dejaría de tener sentido.

-- ── 1 · El tipo ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE documento_cxp DROP CONSTRAINT IF EXISTS documento_cxp_tipo_check;
ALTER TABLE documento_cxp ADD CONSTRAINT documento_cxp_tipo_check
    CHECK (tipo IN ('CXP', 'ANTICIPO', 'VIATICOS', 'REINTEGRO', 'INTERNO', 'NOTA_CREDITO'));

COMMENT ON COLUMN documento_cxp.tipo IS
  'CXP: la factura del proveedor. ANTICIPO: plata ya entregada que se netea. NOTA_CREDITO: la que '
  'emite el proveedor y RESTA de lo que se le debe. VIATICOS/REINTEGRO/INTERNO: documentos que no '
  'nacen de un comprobante del proveedor.';

-- ── 2 · La aplicación ───────────────────────────────────────────────────────────────────────
--
-- EL NOMBRE LLEVA EL PREFIJO DEL MÓDULO, Y NO ES ESTILO. La primera versión de esta migración la
-- llamó `nota_credito_aplicacion` a secas, y ESE NOMBRE YA ESTABA TOMADO: la migración 0054 lo usó
-- para las notas de crédito de CxC, con `cargo_id` donde acá va `factura_id`.
--
-- El `CREATE TABLE IF NOT EXISTS` no falló: encontró la tabla de CxC y no hizo nada, en silencio.
-- Lo que reventó fue el índice de abajo, contra una columna que en esa tabla no existe, y la
-- migración quedó marcada como sucia. Es el peor orden posible para descubrirlo — si el índice no
-- hubiera mencionado `factura_id`, CxP habría empezado a escribir en la tabla de CxC.
--
-- Las tablas propias de CxP llevan `cxp_` (cxp_linea, cxp_recepcion, cxp_parametro, cxp_fuente_
-- recepcion). Un módulo nuevo que invente una tabla con nombre genérico va a chocar con otro.
CREATE TABLE IF NOT EXISTS cxp_nota_credito_aplicacion (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nota_id        uuid NOT NULL REFERENCES documento_cxp(id),
    factura_id     uuid NOT NULL REFERENCES documento_cxp(id),

    -- En la moneda de la FACTURA, que es la del neto contra el que se compara. La nota y la
    -- factura tienen que estar en la misma moneda: no se convierte, por la misma razón que en los
    -- anticipos (hallazgo 3.8) — con qué tipo de cambio es una decisión que no está tomada.
    monto          numeric(16,2) NOT NULL CHECK (monto > 0),

    aplicado_por   uuid REFERENCES usuario(id),
    aplicado_en    timestamptz NOT NULL DEFAULT now(),

    -- Se reversa, no se borra: tabla financiera. Igual que anticipo_aplicacion y pago_parcial.
    activo         boolean NOT NULL DEFAULT true,
    reversado_por  uuid REFERENCES usuario(id),
    reversado_en   timestamptz,

    -- La nota no se aplica a sí misma. Es el único disparate que un CHECK de una fila SÍ puede
    -- atajar, y atajarlo acá cuesta nada.
    CONSTRAINT nc_no_se_aplica_a_si_misma CHECK (nota_id <> factura_id)
);

-- La MISMA nota no se aplica dos veces a la MISMA factura: no sería un reparto, sería un registro
-- duplicado que descontaría dos veces lo mismo.
CREATE UNIQUE INDEX IF NOT EXISTS ux_cxp_nc_aplicacion_viva
    ON cxp_nota_credito_aplicacion (empresa_id, nota_id, factura_id) WHERE activo;

-- Las dos preguntas de siempre: «¿cuánto le bajaron a esta factura?» y «¿cuánto queda de esta
-- nota?». La primera entra en el cálculo del neto, así que se consulta en cada listado.
CREATE INDEX IF NOT EXISTS idx_cxp_nc_aplicacion_factura
    ON cxp_nota_credito_aplicacion (empresa_id, factura_id) WHERE activo;
CREATE INDEX IF NOT EXISTS idx_cxp_nc_aplicacion_nota
    ON cxp_nota_credito_aplicacion (empresa_id, nota_id) WHERE activo;

COMMENT ON TABLE cxp_nota_credito_aplicacion IS
  'Cuánto de una nota de crédito del proveedor se descuenta de cada factura. El neto a pagar resta '
  'estas aplicaciones, así que lo que sale al banco ya viene con la rebaja.';
