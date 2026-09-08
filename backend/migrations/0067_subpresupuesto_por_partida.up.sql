-- Subpresupuesto: el presupuesto de un departamento, desglosado por PARTIDA.
--
-- Decisión del usuario (2026-08-20): «hay departamentos con subpresupuestos», subdivididos por
-- partida de gasto. Es el presupuesto por línea que usan las empresas grandes: «Logística puede
-- gastar ₡2M en Combustible y ₡800k en Mantenimiento este mes».
--
-- ── Cómo convive con lo que ya había ─────────────────────────────────────────
--
-- La misma tabla lleva las dos cosas, distinguidas por `clasificacion_id`:
--
--   · NULL          → el presupuesto TOTAL del departamento en ese mes (lo que ya existía).
--   · con valor     → el subpresupuesto de ESA partida dentro del departamento.
--
-- Nada de lo ya cargado cambia de significado: las filas viejas tienen `clasificacion_id` en NULL y
-- siguen siendo el total. Por eso se agrega una columna en vez de crear otra tabla.
--
-- ── Por qué DOS índices únicos parciales y no uno solo ───────────────────────
--
-- En PostgreSQL, `UNIQUE (a, b, c)` con `c` en NULL **no** impide duplicados: NULL nunca es igual a
-- NULL, así que se podrían cargar dos «totales» del mismo departamento y mes y el sistema aceptaría
-- los dos. Se separa en dos índices para que cada caso quede realmente único.
ALTER TABLE presupuesto_departamento
    ADD COLUMN IF NOT EXISTS clasificacion_id uuid REFERENCES clasificacion(id);

COMMENT ON COLUMN presupuesto_departamento.clasificacion_id IS
    'NULL = el presupuesto TOTAL del departamento en ese mes. Con valor = el subpresupuesto de esa partida dentro del departamento.';

-- La restricción vieja abarcaba (empresa, departamento, periodo) y ahora impediría cargar cualquier
-- subpresupuesto: se reemplaza por los dos índices parciales.
ALTER TABLE presupuesto_departamento
    DROP CONSTRAINT IF EXISTS presupuesto_departamento_empresa_id_departamento_id_periodo_key;

CREATE UNIQUE INDEX IF NOT EXISTS uq_presupuesto_total
    ON presupuesto_departamento (empresa_id, departamento_id, periodo)
    WHERE clasificacion_id IS NULL;

CREATE UNIQUE INDEX IF NOT EXISTS uq_presupuesto_partida
    ON presupuesto_departamento (empresa_id, departamento_id, periodo, clasificacion_id)
    WHERE clasificacion_id IS NOT NULL;

CREATE INDEX IF NOT EXISTS idx_presupuesto_clasificacion
    ON presupuesto_departamento (empresa_id, clasificacion_id)
    WHERE clasificacion_id IS NOT NULL;
