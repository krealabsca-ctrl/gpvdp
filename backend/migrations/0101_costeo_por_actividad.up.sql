-- 0100 · Costeo por actividad (ABC): el indirecto se reparte por lo que lo causa.
--
-- ── QUÉ REEMPLAZA, MEDIDO ───────────────────────────────────────────────────────────────────
--
-- En el archivo de costeo el indirecto entra como una línea llamada «Indirectos Global» que
-- tiene cantidad 1 en los once planes y LA CASILLA DE COSTO VACÍA. El monto se escribe directo
-- en el bloque de resultados: ₡102.399 para los planes directos y ₡1.217.580 para los de
-- suscripción. Un mismo «insumo» con dos costos distintos según dónde se use no es un costo, es
-- un ajuste metido a mano, y nadie puede auditar de dónde sale.
--
-- Además las dos hojas de indirectos reparten 30/70 y 45/85: **45 + 85 = 130 %**. Se absorbe más
-- gasto del que existe, o las bases son distintas y el archivo no lo dice.
--
-- ── LAS TRES REGLAS DEL MODELO ──────────────────────────────────────────────────────────────
--
-- 1 · EL GASTO SALE DE BANCOS, NO SE TECLEA.
--     Hay 124 clasificaciones de gasto con ₡336 millones ya clasificados. El reparto se cuelga
--     de esas clasificaciones, así que el indirecto del costeo y el gasto del libro son el mismo
--     número por construcción. Un total tecleado aparte se desincroniza el primer mes.
--
-- 2 · CADA GASTO SE REPARTE ENTRE DRIVERS Y LA SUMA ES 100 %.
--     Un CHECK no puede sumar entre filas, así que la regla la verifica el servicio y la pantalla
--     la muestra. Lo que el esquema sí garantiza es que no haya dos repartos de la misma
--     clasificación al mismo driver con la misma vigencia.
--
-- 3 · LA BASE SE MIDE POR PERÍODO, NO SE PRESUPUESTA.
--     El archivo divide entre «110 planes» y «360 servicios» fijos. Con un divisor presupuestado,
--     si baja la venta SUBE el costo unitario y el modelo pide subir el precio justo cuando el
--     mercado se enfría — el defecto clásico del costeo por absorción. Acá la base de cada
--     período es un dato de ese período.

-- ── 1 · Los drivers ─────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS driver_costo (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    codigo          text NOT NULL,
    nombre          text NOT NULL,
    -- En qué se mide: «servicios», «contratos», «m²·hora», «kilómetros», «recibos».
    unidad          text NOT NULL DEFAULT 'UNIDAD',
    descripcion     text,
    activo          boolean NOT NULL DEFAULT true,
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_driver_empresa_codigo
    ON driver_costo (empresa_id, lower(codigo));

COMMENT ON TABLE driver_costo IS
  'Generadores de costo: lo que CAUSA que un gasto indirecto exista. El reparto se defiende '
  'ante auditoría nombrando el driver, no con un porcentaje suelto.';

-- ── 2 · La base del driver, por período ─────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS driver_base (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    driver_id       uuid NOT NULL REFERENCES driver_costo(id) ON DELETE CASCADE,
    -- Primer día del mes que se mide.
    periodo         date NOT NULL,
    cantidad        numeric(16,4) NOT NULL CHECK (cantidad > 0),
    origen          text NOT NULL DEFAULT 'MANUAL'
                    CHECK (origen IN ('MANUAL','DERIVADO')),
    nota            text,
    creado_por      uuid REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now()
);

-- Una sola medición por driver y período: dos cantidades para el mismo mes es una ambigüedad que
-- ninguna consulta puede resolver sola.
CREATE UNIQUE INDEX IF NOT EXISTS ux_driver_base_periodo
    ON driver_base (empresa_id, driver_id, periodo);

-- ── 3 · Qué gasto se reparte por qué driver ─────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS gasto_driver (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    -- La clasificación de gasto de Bancos. De ahí sale el monto real.
    clasificacion_id uuid NOT NULL REFERENCES clasificacion(id) ON DELETE CASCADE,
    driver_id       uuid NOT NULL REFERENCES driver_costo(id) ON DELETE CASCADE,
    porcentaje      numeric(7,4) NOT NULL CHECK (porcentaje > 0 AND porcentaje <= 100),
    vigente_desde   date NOT NULL,
    nota            text,
    creado_por      uuid REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_gasto_driver_vigencia
    ON gasto_driver (empresa_id, clasificacion_id, driver_id, vigente_desde);
CREATE INDEX IF NOT EXISTS idx_gasto_driver_clasif
    ON gasto_driver (empresa_id, clasificacion_id, vigente_desde DESC);

COMMENT ON TABLE gasto_driver IS
  'Reparto de una clasificación de gasto entre drivers. La suma por clasificación debe dar 100 %; '
  'el servicio lo verifica y la pantalla lo muestra, porque un CHECK no puede sumar entre filas.';

-- ── 4 · Cuánto consume cada plan de cada driver ─────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS plan_driver (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    plan_id         uuid NOT NULL REFERENCES plan_precio(id) ON DELETE CASCADE,
    driver_id       uuid NOT NULL REFERENCES driver_costo(id) ON DELETE CASCADE,
    consumo         numeric(16,4) NOT NULL CHECK (consumo > 0),
    nota            text,
    creado_en       timestamptz NOT NULL DEFAULT now()
);

CREATE UNIQUE INDEX IF NOT EXISTS ux_plan_driver
    ON plan_driver (empresa_id, plan_id, driver_id);

COMMENT ON TABLE plan_driver IS
  'Cuántas unidades del driver consume un plan. Un servicio funerario consume 1 servicio; un '
  'plan de suscripción consume 1 contrato-mes por cada mes de permanencia.';

-- ── 5 · Los permisos ────────────────────────────────────────────────────────────────────────
--
-- El reparto de indirectos mueve el costo de TODO lo que se vende, así que va con el permiso de
-- editar precios y no con uno nuevo: quien puede cambiar un costo ya puede cambiar el reparto.
-- Un permiso aparte daría la ilusión de una barrera que no existe.
