-- RRHH / Nómina (Fase 3) — Etapa 1: fundamentos. Maqueta aprobada 2026-07-22.
-- Reglas de cr-fiscal-compliance: tablas PARAMETRIZABLES Y VERSIONADAS (cargas CCSS, tramos de
-- renta); las comisiones y bonificaciones habituales SON salario (conceptos de sistema con
-- banderas bloqueadas); un ingreso NO salarial exige base legal. Dinero: numeric, nunca float.

-- Empleados (expediente básico; el histórico laboral fino llega en etapas posteriores).
CREATE TABLE empleado (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id          uuid NOT NULL REFERENCES empresa(id),
    nombre              text NOT NULL,
    tipo_identificacion text NOT NULL DEFAULT 'CEDULA' CHECK (tipo_identificacion IN ('CEDULA', 'DIMEX', 'PASAPORTE')),
    identificacion      text NOT NULL,
    email               text,
    telefono            text,
    iban                text,                                -- cuenta para el archivo de pago SINPE
    departamento_id     uuid REFERENCES departamento(id),
    puesto              text,
    fecha_ingreso       date NOT NULL,
    fecha_salida        date,
    salario_base        numeric(14, 2) NOT NULL CHECK (salario_base >= 0),
    jornada             text NOT NULL DEFAULT 'MENSUAL' CHECK (jornada IN ('MENSUAL', 'QUINCENAL', 'SEMANAL', 'HORAS')),
    activo              boolean NOT NULL DEFAULT true,
    creado_en           timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, identificacion)
);
CREATE INDEX idx_empleado_empresa ON empleado (empresa_id) WHERE activo;

-- Parámetros de nómina VERSIONADOS por empresa y año (cargas, renta, políticas del DF).
-- cargas/tramos van en jsonb con los porcentajes como STRING (se parsean a decimal en Go).
CREATE TABLE nomina_parametros (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    anio            int NOT NULL CHECK (anio >= 2024),
    cargas          jsonb NOT NULL, -- [{codigo, nombre, tipo OBRERO|PATRONAL, pct "5.50"}]
    tramos_renta    jsonb NOT NULL, -- {tramos: [{hasta "918000"|null, pct "10"}], credito_hijo, credito_conyuge}
    ins_riesgos_pct numeric(6, 3) NOT NULL DEFAULT 1.000,  -- variable por póliza (decisión DF)
    aplica_ina      boolean NOT NULL DEFAULT true,          -- excepción: <5 empleados no agrícolas
    adelanto_pct    numeric(5, 2) NOT NULL DEFAULT 50 CHECK (adelanto_pct >= 0 AND adelanto_pct <= 100),
    adelanto_base   text NOT NULL DEFAULT 'SALARIO_BASE' CHECK (adelanto_base IN ('SALARIO_BASE', 'BRUTO')),
    redondeo        text NOT NULL DEFAULT 'COLON' CHECK (redondeo IN ('COLON', 'CENTIMO')),
    provision_base  text NOT NULL DEFAULT 'REMUNERACION_TOTAL' CHECK (provision_base IN ('REMUNERACION_TOTAL', 'SALARIO_BASE')),
    creado_en       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, anio)
);

-- Catálogo de conceptos de ingreso/deducción con banderas de afectación.
-- GUARDARRAÍL: los de_sistema (salario, extras, comisiones, bonos habituales) llevan las
-- banderas BLOQUEADAS — son salario por ley; el backend rechaza editarlas. Un ingreso NO
-- afecto a CCSS exige base_legal (viáticos, reembolsos).
CREATE TABLE concepto_nomina (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       uuid NOT NULL REFERENCES empresa(id),
    nombre           text NOT NULL,
    tipo             text NOT NULL CHECK (tipo IN ('INGRESO', 'DEDUCCION')),
    afecta_ccss      boolean NOT NULL DEFAULT true,
    afecta_renta     boolean NOT NULL DEFAULT true,
    afecta_aguinaldo boolean NOT NULL DEFAULT true,
    base_legal       text,
    de_sistema       boolean NOT NULL DEFAULT false,
    activo           boolean NOT NULL DEFAULT true,
    creado_en        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);

-- Deducciones recurrentes por empleado (tags: Asociación, Ahorro, Préstamo, Soda…):
-- cuota fija, saldo con corte automático al llegar a cero, y prioridad de prelación
-- para cuando el neto no alcanza (menor número = se descuenta primero).
CREATE TABLE deduccion_empleado (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id),
    empleado_id    uuid NOT NULL REFERENCES empleado(id),
    concepto_id    uuid NOT NULL REFERENCES concepto_nomina(id),
    etiqueta       text NOT NULL,
    cuota          numeric(14, 2) NOT NULL CHECK (cuota > 0),
    saldo_total    numeric(14, 2),  -- NULL = recurrente sin tope
    saldo_restante numeric(14, 2),
    prioridad      int NOT NULL DEFAULT 100,
    activo         boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_deduccion_empleado ON deduccion_empleado (empleado_id) WHERE activo;
