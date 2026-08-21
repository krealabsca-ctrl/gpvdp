-- RRHH / Nómina — Etapa 4: incapacidades y vacaciones. Pantalla «Incapacidades y
-- Vacaciones» de la maqueta aprobada. Decisiones del Director Financiero (2026-07-29):
--   · Incapacidad por ENFERMEDAD (CCSS): la empresa paga el 50% del salario los primeros
--     3 días; del cuarto en adelante la CCSS gira su subsidio directo al trabajador y la
--     empresa no paga esos días (Reglamento del Seguro de Salud).
--   · Incapacidad por RIESGO DEL TRABAJO (INS): la empresa paga completo el día del
--     accidente; desde el día siguiente el subsidio lo paga el INS (CT art. 236).
--   · Vacaciones: se acumula 1 día por mes trabajado (CT art. 153, lectura de 12 días
--     hábiles al año). El saldo se DERIVA por SQL; no hay tabla de saldos que desincronizar.

CREATE TABLE incapacidad (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id),
    empleado_id   uuid NOT NULL REFERENCES empleado(id),
    entidad       text NOT NULL CHECK (entidad IN ('CCSS', 'INS')),
    fecha_inicio  date NOT NULL,
    dias          int NOT NULL CHECK (dias > 0 AND dias <= 365),
    boleta        text,                                   -- número de boleta CCSS/INS
    observaciones text,
    anulada       boolean NOT NULL DEFAULT false,         -- nunca se borra
    creado_por    uuid,
    creado_en     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_incapacidad_empleado ON incapacidad (empresa_id, empleado_id) WHERE NOT anulada;
CREATE INDEX idx_incapacidad_fecha ON incapacidad (empresa_id, fecha_inicio) WHERE NOT anulada;

CREATE TABLE vacacion (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id),
    empleado_id   uuid NOT NULL REFERENCES empleado(id),
    fecha_inicio  date NOT NULL,
    dias          numeric(6, 2) NOT NULL CHECK (dias > 0 AND dias <= 365),
    observaciones text,
    anulada       boolean NOT NULL DEFAULT false,
    creado_por    uuid,
    creado_en     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_vacacion_empleado ON vacacion (empresa_id, empleado_id) WHERE NOT anulada;

-- Días de vacaciones que se acumulan por mes trabajado (parametrizable por empresa/año).
ALTER TABLE nomina_parametros ADD COLUMN vacaciones_dias_mes numeric(5, 2) NOT NULL DEFAULT 1.00;
COMMENT ON COLUMN nomina_parametros.vacaciones_dias_mes IS
    'Días de vacaciones que acumula el empleado por cada mes trabajado (CT art. 153: 1 = 12 días hábiles/año)';
