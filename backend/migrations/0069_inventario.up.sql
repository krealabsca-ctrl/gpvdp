-- Módulo de Inventario · Fase 1: catálogo, existencias, entradas, salidas y traslados.
--
-- Propuesta aprobada por el usuario el 2026-08-22 (docs/GPVDP_Inventario_Propuesta_v1.0.md).
-- Volumen declarado: 40–50 unidades hoy, hasta 100–150 entre sedes; urnas semanales.
--
-- ── Las dos decisiones que explican todo lo de abajo ─────────────────────────
--
--  1. **Dos formas de contar.** El cofre se controla POR UNIDAD (cada uno tiene costo, sede y
--     estado propios: valen entre ₡70.000 y ₡423.750 según las facturas reales); la urna se
--     controla POR CANTIDAD (se pide cada semana y no tiene identidad). Un solo mecanismo
--     obligaría a elegir entre perder la trazabilidad del cofre o llenar de burocracia la urna.
--
--  2. **La existencia se DERIVA del libro de movimientos, no se guarda.** Es el mismo principio
--     que el saldo diario en Bancos. Un campo `cantidad_actual` que se actualiza a mano se
--     desincroniza, y entonces hay dos verdades y nadie sabe cuál es la buena.

-- ── Categorías (árbol de un nivel: categoría › subcategoría) ────────────────
CREATE TABLE IF NOT EXISTS inv_categoria (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    padre_id      uuid REFERENCES inv_categoria(id),
    nombre        text NOT NULL,
    activo        boolean NOT NULL DEFAULT true,
    orden         integer NOT NULL DEFAULT 0,
    creado_en     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX IF NOT EXISTS idx_inv_categoria_empresa ON inv_categoria (empresa_id, activo);

-- La unicidad va en DOS índices parciales y no en un `UNIQUE (empresa_id, padre_id, nombre)`.
--
-- Motivo, verificado con una prueba que creó dos «Cofres» de primer nivel: en PostgreSQL NULL nunca
-- es igual a NULL, así que una restricción que incluya `padre_id` NO impide repetir el nombre entre
-- las categorías RAÍZ —que son justamente las que el usuario ve primero—. Es la misma trampa de las
-- migraciones 0066 y 0067.
CREATE UNIQUE INDEX IF NOT EXISTS uq_inv_categoria_raiz
    ON inv_categoria (empresa_id, nombre)
    WHERE padre_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_inv_categoria_hija
    ON inv_categoria (empresa_id, padre_id, nombre)
    WHERE padre_id IS NOT NULL;

-- ── Artículos ───────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS inv_articulo (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    categoria_id    uuid NOT NULL REFERENCES inv_categoria(id),
    codigo          text NOT NULL,
    nombre          text NOT NULL,
    -- UNIDAD = una fila por objeto físico en inv_unidad. CANTIDAD = solo se cuenta.
    modo_control    text NOT NULL CHECK (modo_control IN ('UNIDAD', 'CANTIDAD')),
    unidad_medida   text NOT NULL DEFAULT 'unidad',
    proveedor_id    uuid REFERENCES proveedor(id),
    -- La partida con la que se registra su compra: enlaza el inventario con el gasto ya
    -- clasificado, sin que el inventario dependa de la contabilidad para saber qué hay.
    clasificacion_id uuid,
    activo          boolean NOT NULL DEFAULT true,
    nota            text,
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now(),
    creado_por      uuid REFERENCES usuario(id),
    UNIQUE (empresa_id, codigo)
);
CREATE INDEX IF NOT EXISTS idx_inv_articulo_empresa ON inv_articulo (empresa_id, activo);
CREATE INDEX IF NOT EXISTS idx_inv_articulo_categoria ON inv_articulo (empresa_id, categoria_id);

COMMENT ON COLUMN inv_articulo.modo_control IS
    'UNIDAD: cada objeto físico es una fila en inv_unidad (cofres). CANTIDAD: solo se cuenta (urnas, suministros).';

-- ── Mínimos y máximos por artículo y sede ───────────────────────────────────
--
-- Van en su propia tabla y no como columnas del artículo porque el mínimo de una urna en Cartago
-- no tiene nada que ver con el de Sabana: es una decisión por sede.
CREATE TABLE IF NOT EXISTS inv_nivel (
    empresa_id   uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    articulo_id  uuid NOT NULL REFERENCES inv_articulo(id) ON DELETE CASCADE,
    sede_id      uuid NOT NULL REFERENCES sede(id) ON DELETE CASCADE,
    minimo       integer NOT NULL DEFAULT 0 CHECK (minimo >= 0),
    maximo       integer NOT NULL DEFAULT 0 CHECK (maximo >= 0),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    PRIMARY KEY (empresa_id, articulo_id, sede_id),
    CONSTRAINT inv_nivel_maximo_coherente CHECK (maximo = 0 OR maximo >= minimo)
);

-- ── Unidades: una fila por objeto físico (solo modo UNIDAD) ─────────────────
CREATE TABLE IF NOT EXISTS inv_unidad (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    articulo_id     uuid NOT NULL REFERENCES inv_articulo(id),
    numero          text NOT NULL,
    -- Dónde está. NULL mientras va en tránsito entre dos sedes.
    sede_id         uuid REFERENCES sede(id),
    sede_destino_id uuid REFERENCES sede(id),
    estado          text NOT NULL DEFAULT 'DISPONIBLE'
                    CHECK (estado IN ('DISPONIBLE','RESERVADA','EN_TRANSITO','EXHIBICION',
                                      'USADA','DANADA','DEVUELTA')),
    costo_crc       numeric(14,2) NOT NULL DEFAULT 0 CHECK (costo_crc >= 0),
    -- Consignada = el proveedor la dejó acá y se le paga cuando se usa. NO es capital propio.
    es_consignada   boolean NOT NULL DEFAULT false,
    proveedor_id    uuid REFERENCES proveedor(id),
    documento_cxp_id uuid REFERENCES documento_cxp(id),
    ingresada_en    date NOT NULL,
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, numero)
);
CREATE INDEX IF NOT EXISTS idx_inv_unidad_ubicacion ON inv_unidad (empresa_id, sede_id, estado);
CREATE INDEX IF NOT EXISTS idx_inv_unidad_articulo ON inv_unidad (empresa_id, articulo_id, estado);

COMMENT ON COLUMN inv_unidad.sede_id IS
    'Dónde está la unidad. NULL solo mientras el estado es EN_TRANSITO: salió del origen y todavía nadie la recibió en el destino.';

-- ── Servicios prestados ─────────────────────────────────────────────────────
--
-- El hecho que descarga el inventario, y que hoy no se registraba en ninguna parte (CxC lleva la
-- cartera de asociados, que es otra cosa). Sin esto el inventario sería un contador manual.
CREATE TABLE IF NOT EXISTS inv_servicio (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    numero        text NOT NULL,
    sede_id       uuid REFERENCES sede(id),
    fecha         date NOT NULL,
    a_nombre_de   text NOT NULL DEFAULT '',
    contrato_id   uuid REFERENCES contrato_cxc(id),
    nota          text,
    creado_en     timestamptz NOT NULL DEFAULT now(),
    creado_por    uuid REFERENCES usuario(id),
    UNIQUE (empresa_id, numero)
);
CREATE INDEX IF NOT EXISTS idx_inv_servicio_fecha ON inv_servicio (empresa_id, fecha DESC);

-- ── El libro de movimientos ─────────────────────────────────────────────────
--
-- Append-only: de acá sale toda existencia. Cada fila dice qué se movió, cuánto, de dónde y a
-- dónde. El signo lo determina el tipo, no quien escribe.
CREATE TABLE IF NOT EXISTS inv_movimiento (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    articulo_id   uuid NOT NULL REFERENCES inv_articulo(id),
    unidad_id     uuid REFERENCES inv_unidad(id),
    -- El ajuste va en DOS tipos y no en uno con cantidad negativa: la cantidad siempre es positiva
    -- y el sentido lo da el tipo, así ninguna consulta tiene que acordarse del signo.
    tipo          text NOT NULL CHECK (tipo IN ('ENTRADA','SALIDA','TRASLADO_SALIDA','TRASLADO_ENTRADA',
                                               'AJUSTE_MAS','AJUSTE_MENOS','BAJA','DEVOLUCION')),
    -- Siempre positiva: el sentido lo da el tipo. Guardar cantidades negativas obliga a que cada
    -- consulta recuerde el signo, y basta que una se olvide para que el total mienta.
    cantidad      integer NOT NULL CHECK (cantidad > 0),
    sede_id       uuid REFERENCES sede(id),
    sede_contra_id uuid REFERENCES sede(id),
    costo_unitario_crc numeric(14,2) NOT NULL DEFAULT 0 CHECK (costo_unitario_crc >= 0),
    fecha         date NOT NULL,
    servicio_id   uuid REFERENCES inv_servicio(id),
    proveedor_id  uuid REFERENCES proveedor(id),
    documento_cxp_id uuid REFERENCES documento_cxp(id),
    traslado_id   uuid,
    motivo        text,
    creado_en     timestamptz NOT NULL DEFAULT now(),
    creado_por    uuid REFERENCES usuario(id)
);
CREATE INDEX IF NOT EXISTS idx_inv_mov_articulo ON inv_movimiento (empresa_id, articulo_id, fecha);
CREATE INDEX IF NOT EXISTS idx_inv_mov_sede ON inv_movimiento (empresa_id, sede_id, fecha);
CREATE INDEX IF NOT EXISTS idx_inv_mov_servicio ON inv_movimiento (empresa_id, servicio_id)
    WHERE servicio_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_inv_mov_traslado ON inv_movimiento (empresa_id, traslado_id)
    WHERE traslado_id IS NOT NULL;

COMMENT ON COLUMN inv_movimiento.cantidad IS
    'Siempre positiva. El sentido (suma o resta) lo determina el tipo del movimiento.';
COMMENT ON COLUMN inv_movimiento.traslado_id IS
    'Une la salida y la entrada de un mismo traslado. Mientras solo exista la salida, lo trasladado está en tránsito.';

-- ── Traslados entre sedes ───────────────────────────────────────────────────
--
-- Tabla propia y no solo un par de movimientos, porque un traslado tiene estado propio: sale,
-- viaja y alguien lo recibe. Es lo que evita que una unidad extraviada en el camino desaparezca
-- del sistema sin que nadie responda por ella.
CREATE TABLE IF NOT EXISTS inv_traslado (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    numero         text NOT NULL,
    sede_origen_id uuid NOT NULL REFERENCES sede(id),
    sede_destino_id uuid NOT NULL REFERENCES sede(id),
    estado         text NOT NULL DEFAULT 'EN_TRANSITO'
                   CHECK (estado IN ('EN_TRANSITO','RECIBIDO','CANCELADO')),
    enviado_en     date NOT NULL,
    recibido_en    date,
    enviado_por    uuid REFERENCES usuario(id),
    recibido_por   uuid REFERENCES usuario(id),
    nota           text,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, numero),
    CONSTRAINT inv_traslado_recibido_coherente
        CHECK ((estado = 'RECIBIDO') = (recibido_en IS NOT NULL)),
    CONSTRAINT inv_traslado_sedes_distintas
        CHECK (sede_origen_id <> sede_destino_id)
);
CREATE INDEX IF NOT EXISTS idx_inv_traslado_estado ON inv_traslado (empresa_id, estado, enviado_en);

-- Consecutivos por empresa para servicios y traslados (mismo patrón que el resto del sistema).
CREATE TABLE IF NOT EXISTS inv_consecutivo (
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    ambito     text NOT NULL,
    siguiente  integer NOT NULL DEFAULT 1,
    PRIMARY KEY (empresa_id, ambito)
);
