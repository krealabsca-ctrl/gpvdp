-- Fase C — Proyecciones (CU-10): escenarios de cierre de mes guardados.
-- Cada escenario congela método + meta + resultado al día del cálculo; luego se
-- compara contra el real del cierre (KPI "precisión de proyección", spec §21).
CREATE TABLE proyeccion_escenario (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id           uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    periodo              text NOT NULL, -- YYYY-MM proyectado
    metodo               text NOT NULL CHECK (metodo IN ('RITMO','HISTORICO','PROMEDIO','COINCIDENCIA')),
    -- Si el método pedido no tenía histórico suficiente, se cayó a RITMO.
    metodo_efectivo      text NOT NULL,
    meta_crecimiento_pct numeric(6,2) NOT NULL DEFAULT 0,
    lineas_ingreso       text[] NOT NULL DEFAULT '{}',
    dia_calculo          int NOT NULL,
    real_acumulado       numeric(18,2) NOT NULL,
    cierre_proyectado    numeric(18,2) NOT NULL,
    meta_monto           numeric(18,2) NOT NULL,
    creado_por           uuid,
    creado_en            timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_proyeccion_empresa_periodo ON proyeccion_escenario (empresa_id, periodo);
