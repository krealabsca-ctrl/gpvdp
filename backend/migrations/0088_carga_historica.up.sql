-- Cargar el HISTÓRICO: un archivo, varias cuentas, varios meses.
--
-- ── DE DÓNDE SALE ───────────────────────────────────────────────────────────
--
-- Textual del Director Financiero: «lo que no se puede hacer es buscar todos los bancos de estos
-- años e ir de uno en uno cargandolo». Tiene 2025 y 2026 en Excel, YA SEGMENTADOS por él, y el
-- importador de siempre pide UNA cuenta y UN archivo por vez: 15 cuentas × 24 meses = 360 subidas.
--
-- Su primer intento fue por «Traer la clasificación desde Excel», que SOLO pinta la partida sobre
-- movimientos ya cargados y nunca los crea: como no había nada anterior al 01/07/2026, todas sus
-- filas salieron «ese movimiento no está cargado». Esta tabla es la mitad que faltaba.
--
-- ── POR QUÉ UNA TABLA PROPIA Y NO UNA `importacion` ─────────────────────────
--
-- `importacion.cuenta_bancaria_id` es NOT NULL, y eso es correcto: una carga normal ES de una
-- cuenta. Pero acá el archivo todavía no se repartió en cuentas cuando se sube: primero se
-- PREVISUALIZA (qué cuentas trae, cuántas filas cada una, qué se entendió y qué no) y recién al
-- confirmar se crea UNA importación POR CUENTA. Meter el archivo en una `importacion` prematura
-- obligaría a inventarle una cuenta, o a guardar los bytes 15 veces.
--
-- Guardar el archivo es lo que permite confirmar sin volver a subirlo —igual que RN-06 en el
-- importador de siempre— y además deja la prueba de qué se cargó si mañana alguien pregunta.
--
-- Las importaciones que nacen al confirmar comparten `nombre_archivo` y `source_file_hash`, así el
-- panel «Cargas hechas» las muestra juntas y CADA UNA SE PUEDE REVERTIR SOLA: esa es la salida si
-- el Director se equivoca en UNA cuenta de las quince.

CREATE TABLE carga_historica (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre_archivo   text NOT NULL,
    source_file_hash text NOT NULL,
    -- El archivo original tal cual se subió. NOT NULL: una carga sin archivo no se podría
    -- confirmar y quedaría de adorno ocupando un id que la pantalla muestra.
    archivo          bytea NOT NULL,
    estado           text NOT NULL DEFAULT 'PREVISUALIZADA'
                     CHECK (estado IN ('PREVISUALIZADA', 'CONFIRMADA')),
    creado_por       uuid REFERENCES usuario(id),
    creado_en        timestamptz NOT NULL DEFAULT now(),
    confirmada_en    timestamptz,
    -- El estado y la fecha cuentan la misma historia o ninguna de las dos sirve.
    CONSTRAINT carga_historica_confirmacion_coherente CHECK (
        (estado = 'CONFIRMADA') = (confirmada_en IS NOT NULL)
    )
);

CREATE INDEX idx_carga_historica_empresa ON carga_historica (empresa_id, creado_en DESC);
