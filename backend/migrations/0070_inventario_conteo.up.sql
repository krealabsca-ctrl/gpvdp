-- Inventario · Fase 2: el conteo cíclico.
--
-- No se para la operación para un inventario general: se cuenta por partes, y lo caro más seguido
-- que lo barato (§2 de docs/GPVDP_Inventario_Propuesta_v1.0.md). La regla que gobierna todo esto:
-- **la diferencia se muestra y se explica; nunca se ajusta en silencio.**
--
-- ── Las dos decisiones que explican el modelo ────────────────────────────────
--
--  1. **La cantidad del sistema se CONGELA al abrir la hoja.** Es la foto contra la que se compara.
--     Si se recalculara al cerrar, un movimiento hecho durante el conteo cambiaría la diferencia y
--     nadie podría explicar por qué: el conteo dejaría de ser una medición y pasaría a ser una
--     carrera contra la operación.
--
--  2. **Al cerrar, cada diferencia explicada genera su movimiento de ajuste.** Sin eso el conteo
--     sería un informe que alguien tiene que copiar a mano a los ajustes, y el sistema seguiría
--     diciendo un número que la bodega ya desmintió.

CREATE TABLE IF NOT EXISTS inv_conteo (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    numero        text NOT NULL,
    sede_id       uuid NOT NULL REFERENCES sede(id),
    -- Qué se contó: una categoría concreta o toda la sede (NULL).
    categoria_id  uuid REFERENCES inv_categoria(id),
    estado        text NOT NULL DEFAULT 'ABIERTO'
                  CHECK (estado IN ('ABIERTO', 'CERRADO', 'ANULADO')),
    abierto_en    date NOT NULL,
    cerrado_en    date,
    abierto_por   uuid REFERENCES usuario(id),
    cerrado_por   uuid REFERENCES usuario(id),
    nota          text,
    creado_en     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, numero),
    -- Un conteo cerrado tiene fecha de cierre, y uno abierto no: si se pudieran separar, una hoja
    -- «abierta» con fecha de cierre no sería ni una cosa ni la otra.
    CONSTRAINT inv_conteo_cierre_coherente
        CHECK ((estado = 'CERRADO') = (cerrado_en IS NOT NULL))
);
CREATE INDEX IF NOT EXISTS idx_inv_conteo_sede ON inv_conteo (empresa_id, sede_id, abierto_en DESC);

-- Solo UNA hoja abierta por sede a la vez. Dos personas contando la misma bodega con dos fotos
-- distintas del sistema producen dos verdades y ningún ajuste confiable.
CREATE UNIQUE INDEX IF NOT EXISTS uq_inv_conteo_abierto_por_sede
    ON inv_conteo (empresa_id, sede_id)
    WHERE estado = 'ABIERTO';

COMMENT ON COLUMN inv_conteo.categoria_id IS
    'Categoría contada. NULL = se contó toda la sede.';

-- ── Las líneas de la hoja ───────────────────────────────────────────────────
--
-- Una fila por artículo (modo CANTIDAD) o por unidad física (modo UNIDAD). Son dos formas de contar
-- porque son dos preguntas distintas: «¿cuántas urnas hay?» y «¿está este cofre?».
CREATE TABLE IF NOT EXISTS inv_conteo_linea (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    conteo_id     uuid NOT NULL REFERENCES inv_conteo(id) ON DELETE CASCADE,
    articulo_id   uuid NOT NULL REFERENCES inv_articulo(id),
    -- Para modo UNIDAD: qué ficha se está buscando.
    unidad_id     uuid REFERENCES inv_unidad(id),
    -- La foto del sistema al ABRIR la hoja. No se toca después.
    cantidad_sistema integer NOT NULL,
    -- Lo contado en bodega. NULL = todavía nadie lo contó, que es distinto de «contó cero».
    cantidad_contada integer,
    -- La explicación de la diferencia. Sin ella el conteo no se puede cerrar.
    motivo        text,
    contado_en    timestamptz,
    contado_por   uuid REFERENCES usuario(id),
    UNIQUE (empresa_id, conteo_id, articulo_id, unidad_id),
    CONSTRAINT inv_conteo_linea_cantidades_validas
        CHECK (cantidad_sistema >= 0 AND (cantidad_contada IS NULL OR cantidad_contada >= 0))
);
CREATE INDEX IF NOT EXISTS idx_inv_conteo_linea ON inv_conteo_linea (empresa_id, conteo_id);

COMMENT ON COLUMN inv_conteo_linea.cantidad_sistema IS
    'Lo que el sistema decía al ABRIR la hoja. Congelado a propósito: si se recalculara al cerrar, un movimiento hecho durante el conteo cambiaría la diferencia sin explicación.';
COMMENT ON COLUMN inv_conteo_linea.cantidad_contada IS
    'Lo contado en bodega. NULL = sin contar todavía; 0 = se contó y no había ninguno. Son cosas distintas.';

-- La unicidad de la línea con `unidad_id` en NULL (modo CANTIDAD) necesita su índice parcial: un
-- UNIQUE que incluya una columna nullable no impide duplicados, porque NULL nunca es igual a NULL.
-- Es la misma trampa de las migraciones 0066, 0067 y 0068.
CREATE UNIQUE INDEX IF NOT EXISTS uq_inv_conteo_linea_articulo
    ON inv_conteo_linea (empresa_id, conteo_id, articulo_id)
    WHERE unidad_id IS NULL;
