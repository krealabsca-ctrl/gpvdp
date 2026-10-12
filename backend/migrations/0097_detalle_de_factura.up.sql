-- 0096 · El detalle de la factura: qué se compró, no solo cuánto se pagó.
--
-- POR QUÉ. Las 4.533 facturas del sistema traen el ENCABEZADO (proveedor, fecha, total, clave) y
-- nada más: entraron por el importador de Excel. El XML de Hacienda sí trae, línea por línea, el
-- código CABYS, la descripción, la cantidad, la unidad y el precio unitario — y el parser del
-- repo YA los lee (`lineaXML` en xml_hacienda.go), pero nadie los guardaba: el visor de
-- comprobante los arma al vuelo para UNA factura y los tira. Sin una tabla no hay forma de
-- preguntar "¿cuánto gastamos en esto?" ni "¿a qué precio nos lo vendió cada proveedor?".
--
-- LOS MONTOS VAN EN LA MONEDA DE LA FACTURA, sin convertir. La conversión a colones se hace AL
-- LEER, multiplicando por `documento_cxp.tc_aplicado`, que es el tipo de cambio de esa factura.
-- Guardar acá un monto ya convertido congelaría un TC en una fila que nadie volvería a mirar.
--
-- NULL NO ES CERO. Un elemento que el comprobante no trajo queda en NULL; uno que vino en cero
-- queda en 0. La diferencia importa: un precio unitario ausente no se puede comparar contra nada,
-- y un precio unitario de cero es un regalo o un error del emisor. Un `0` en lugar de NULL
-- metería ambos en el mismo promedio.
CREATE TABLE cxp_linea (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    documento_id  uuid NOT NULL REFERENCES documento_cxp(id) ON DELETE CASCADE,

    -- `linea` es el ORDINAL de aparición (1..N), no lo que diga el comprobante: es lo que sostiene
    -- la clave única. `numero_linea` guarda lo que el XML declaró, tal cual, porque un emisor que
    -- numera mal (repetido, saltado, vacío) no puede impedir que la factura se cargue.
    linea         integer NOT NULL,
    numero_linea  text    NOT NULL DEFAULT '',

    cabys            text NOT NULL DEFAULT '',
    codigo_comercial text NOT NULL DEFAULT '',
    detalle          text NOT NULL DEFAULT '',

    -- `detalle_norm` es la descripción llevada a una forma comparable (sin tildes, sin signos,
    -- minúsculas, espacios colapsados). Sirve para juntar "TORNILLO 1/2" con "tornillo 1/2".
    -- Se escribe en la ingesta y no es columna generada a propósito: la regla va a mejorar, y
    -- recargar la carpeta de XML es idempotente, así que afinarla no obliga a una migración.
    detalle_norm     text NOT NULL DEFAULT '',

    cantidad          numeric(18,5),
    unidad_medida     text NOT NULL DEFAULT '',
    precio_unitario   numeric(18,5),
    monto_total       numeric(18,5),
    descuento         numeric(18,5),
    subtotal          numeric(18,5),
    impuesto          numeric(18,5),
    monto_total_linea numeric(18,5),

    creado_en timestamptz NOT NULL DEFAULT now(),

    -- Recargar el mismo XML REESCRIBE sus líneas, no las duplica.
    UNIQUE (documento_id, linea)
);

-- El análisis entra por CABYS dentro de una empresa; el visor de una factura entra por documento.
CREATE INDEX idx_cxp_linea_cabys     ON cxp_linea (empresa_id, cabys) WHERE cabys <> '';
CREATE INDEX idx_cxp_linea_documento ON cxp_linea (documento_id);
CREATE INDEX idx_cxp_linea_norm      ON cxp_linea (empresa_id, detalle_norm) WHERE detalle_norm <> '';

-- Dejar constancia en la factura de que su detalle ya se cargó, sin tener que contar filas de otra
-- tabla para pintar una lista.
ALTER TABLE documento_cxp
    ADD COLUMN IF NOT EXISTS detalle_lineas integer NOT NULL DEFAULT 0;

-- Permiso para la pantalla de análisis de gasto. La CARGA del detalle no estrena permiso: es
-- cargar documentos, y eso ya es `cxp.importar`.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('cxp.gasto',
     'Cuentas por pagar',
     'Analizar el gasto por ítem',
     'Qué se compra, a qué precio y a cuántos proveedores, según el detalle (CABYS) del XML de cada factura.')
ON CONFLICT (codigo) DO NOTHING;

-- Se siembra recorriendo las parejas (empresa, rol) QUE YA EXISTEN en `rol_permiso`: los roles
-- base son globales (`empresa_id` NULL) y `rol_permiso` lo exige NOT NULL. Lo recibe quien ya
-- podía ver el tablero de CxP, que es la misma audiencia de un análisis de gasto.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso dash ON dash.id = rp.permiso_id AND dash.codigo = 'cxp.dashboard'
CROSS JOIN permiso p
WHERE p.codigo = 'cxp.gasto'
ON CONFLICT DO NOTHING;
