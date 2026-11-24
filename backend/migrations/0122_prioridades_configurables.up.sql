-- Las prioridades de pago dejan de estar escritas en el código (11-nov-2026).
--
-- «Que se pueda agregar nuevas prioridades.»
--
-- Hasta hoy la prioridad era un CHECK con tres valores —'', 'A', 'AA'— y agregar una cuarta
-- obligaba a una migración y un despliegue. Es el mismo problema que tenían los formatos de banco
-- antes de la mig 0108: una decisión del negocio que necesitaba un programador.
--
-- DOS CAMPOS, NINGUNA MAGIA. El orden y la urgencia se declaran por separado a propósito:
--
--   `orden`    manda en cómo se ORDENAN las listas (mayor = primero). Es un número, así que meter
--              una prioridad entre dos existentes no obliga a renumerar nada.
--   `urgente`  dice si esa prioridad cuenta en «lo que se paga sí o sí»: el tablero, el filtro de
--              urgentes y la premarca del corte. Si fuera derivado del orden —«la más alta es la
--              urgente»— crear una prioridad nueva por encima dejaría de contar la anterior en
--              silencio, y el tablero mostraría menos cartera urgente sin que nadie tocara una
--              factura.
--
-- EL DOCUMENTO SIGUE GUARDANDO EL CÓDIGO, no un id. Es lo que permite que una prioridad retirada
-- no rompa las facturas que ya la llevaban: el código queda escrito, y el catálogo decide cómo se
-- muestra y si se puede volver a usar. Mismo criterio que el resto del catálogo (ver
-- `gpvdp-catalogo-correcciones`): lo que cuelga no se borra, se desactiva.

CREATE TABLE prioridad_cxp (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id),
    -- El código es lo que queda escrito en la factura. Corto porque se muestra en una insignia
    -- dentro de una fila de tabla.
    codigo  text NOT NULL,
    nombre  text NOT NULL,
    orden   integer NOT NULL DEFAULT 0,
    urgente boolean NOT NULL DEFAULT false,
    -- El tono de la insignia, de la paleta que ya usa la interfaz. Se guarda el NOMBRE del tono y
    -- no un color suelto: así una prioridad nueva se ve como el resto del sistema y sigue el tema
    -- claro/oscuro sin que nadie elija un hexadecimal que después no contrasta.
    tono   text NOT NULL DEFAULT 'pendiente',
    activa boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    CONSTRAINT ux_prioridad_codigo UNIQUE (empresa_id, codigo),
    CONSTRAINT ck_prioridad_codigo CHECK (length(btrim(codigo)) BETWEEN 1 AND 6),
    CONSTRAINT ck_prioridad_nombre CHECK (length(btrim(nombre)) >= 3),
    CONSTRAINT ck_prioridad_tono CHECK (tono IN ('neutral', 'accent', 'positivo', 'negativo', 'pendiente'))
);

CREATE INDEX idx_prioridad_empresa ON prioridad_cxp (empresa_id, orden DESC);

-- Las dos que ya existían, para CADA empresa, con el significado que tienen hoy en la Bandeja:
-- «AA — se paga sí o sí» y «A — puede esperar». Se siembran con hueco entre los órdenes (10 y 20)
-- para que meter una intermedia después no obligue a renumerar.
INSERT INTO prioridad_cxp (empresa_id, codigo, nombre, orden, urgente, tono)
SELECT e.id, v.codigo, v.nombre, v.orden, v.urgente, v.tono
  FROM empresa e
 CROSS JOIN (VALUES
        ('AA', 'Se paga sí o sí', 20, true,  'negativo'),
        ('A',  'Puede esperar',   10, false, 'pendiente')
    ) AS v(codigo, nombre, orden, urgente, tono)
ON CONFLICT DO NOTHING;

-- Fuera el candado que impedía agregar prioridades. La validación pasa al servicio, que comprueba
-- contra este catálogo: ahí sí puede decir CUÁLES son las válidas, que es lo que un CHECK no sabe
-- responder cuando el usuario escribe una que no existe.
ALTER TABLE documento_cxp DROP CONSTRAINT IF EXISTS documento_cxp_prioridad_check;

COMMENT ON TABLE prioridad_cxp IS
    'Prioridades de pago por empresa. `orden` manda en el orden de las listas (mayor = primero); `urgente` dice si cuenta en «lo que se paga sí o sí». Los dos se declaran aparte: derivar la urgencia del orden haría que crear una prioridad nueva dejara de contar la anterior sin avisar.';
