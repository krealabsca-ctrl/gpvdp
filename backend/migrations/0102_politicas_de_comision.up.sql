-- 0101 · Políticas de comisión: por plan y por vendedor, con su carga social.
--
-- ── POR QUÉ CONFIGURABLE Y NO UN NÚMERO EN EL COSTEO ────────────────────────────────────────
--
-- Decisión del Director Financiero (14-oct-2026): «las políticas de comisión son por plan y por
-- vendedor, esto de momento no es posible porque es variable, entonces debe ser un espacio
-- configurable».
--
-- Los datos lo confirman. El gasto de comisiones que sale por banco es irregular: ₡28.620.309 en
-- agosto de 2026 contra ₡150.000 en casi todos los demás meses. Un solo mes concentra el 87 % del
-- total, así que no hay un porcentaje estable que derivar. Lo que sí es un dato firme, y sirve de
-- control: ₡33.050.757 pagados entre abril de 2025 y septiembre de 2026.
--
-- ── EL HALLAZGO QUE ESTA TABLA CORRIGE ──────────────────────────────────────────────────────
--
-- En nómina, el concepto «Comisiones» está marcado como afecta CCSS, afecta renta y afecta
-- aguinaldo en las tres empresas. Está bien puesto: es lo que manda la ley, las comisiones
-- habituales son salario.
--
-- Pero el costeo no lo refleja. El plan Oro lleva ₡135.000 de comisión y el costo real para la
-- empresa es ₡135.000 × 1,2683 = ₡171.221 con las cargas patronales. Son ₡36.221 por venta que
-- hoy no están en el costo, y el error va en todos los planes directos. En los de suscripción el
-- problema es el inverso: la comisión figura en CERO.
--
-- Por eso el factor de carga NO se teclea en cada política: se toma de los parámetros de nómina,
-- que ya están versionados por año. Si cada quien tuviera que acordarse de multiplicar, basta que
-- una política se olvide para que ese plan quede costeado de menos.
--
-- ── LA REGLA DE RESOLUCIÓN ──────────────────────────────────────────────────────────────────
--
-- De lo específico a lo general: política de ESTE vendedor con ESTE plan → política del plan →
-- política del vendedor → política general. Es el mismo patrón con que el sistema ya resuelve la
-- clasificación por defecto de un gasto.
--
-- `plan_id` y `vendedor_id` en NULL significan «cualquiera». No se usa un valor centinela porque
-- un uuid falso obligaría a recordarlo en cada consulta.

CREATE TABLE IF NOT EXISTS politica_comision (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),

    -- NULL = aplica a todos los planes / a todos los vendedores.
    plan_id         uuid REFERENCES plan_precio(id) ON DELETE CASCADE,
    vendedor_id     uuid REFERENCES empleado(id) ON DELETE CASCADE,
    -- Canal o grupo de venta, cuando la política no es de una persona sino de un equipo.
    canal           text,

    tipo            text NOT NULL DEFAULT 'PORCENTAJE'
                    CHECK (tipo IN ('PORCENTAJE','MONTO_FIJO')),

    -- Sobre qué se calcula. PRECIO es lo habitual; MARGEN alinea al vendedor con la
    -- rentabilidad y no con el volumen; PRIMERA_CUOTA es lo que se usa en suscripción, donde
    -- comisionar sobre el total del contrato pagaría por adelantado 36 meses que quizá no
    -- lleguen.
    base            text NOT NULL DEFAULT 'PRECIO'
                    CHECK (base IN ('PRECIO','MARGEN','PRIMERA_CUOTA')),

    valor           numeric(16,4) NOT NULL CHECK (valor >= 0),

    vigente_desde   date NOT NULL,
    vigente_hasta   date,
    CHECK (vigente_hasta IS NULL OR vigente_hasta >= vigente_desde),

    notas           text,
    activo          boolean NOT NULL DEFAULT true,
    creado_por      uuid REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now()
);

-- Una sola política por combinación y fecha de vigencia. El COALESCE con el uuid nulo es lo que
-- permite que el índice único trate «todos los planes» como un valor más: sin él, dos políticas
-- generales con la misma vigencia entrarían las dos y el costeo elegiría una al azar.
CREATE UNIQUE INDEX IF NOT EXISTS ux_politica_comision
    ON politica_comision (
        empresa_id,
        COALESCE(plan_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(vendedor_id, '00000000-0000-0000-0000-000000000000'::uuid),
        COALESCE(canal, ''),
        vigente_desde
    );

CREATE INDEX IF NOT EXISTS idx_politica_comision_plan
    ON politica_comision (empresa_id, plan_id, vigente_desde DESC) WHERE activo;

COMMENT ON TABLE politica_comision IS
  'Cuánto se le paga a quién por vender qué. La carga social NO se guarda acá: se toma de los '
  'parámetros de nómina al calcular, para que ninguna política pueda olvidarse de aplicarla.';

COMMENT ON COLUMN politica_comision.base IS
  'PRECIO: sobre el precio de venta. MARGEN: sobre la contribución, alinea al vendedor con la '
  'rentabilidad. PRIMERA_CUOTA: para suscripción, evita pagar por adelantado meses que no llegaron.';
