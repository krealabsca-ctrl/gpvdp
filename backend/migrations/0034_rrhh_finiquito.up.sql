-- RRHH / Nómina (Fase 3) — Etapa 3: finiquito (liquidación de cese conforme al Código de
-- Trabajo) y bitácora del archivo de pago SINPE (consecutivo por empresa, cuadre 1:1 con
-- la corrida). Maqueta aprobada: pantallas "Liquidación / Prestaciones" y "Reportes y SINPE".

CREATE TABLE finiquito (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       uuid NOT NULL REFERENCES empresa(id),
    empleado_id      uuid NOT NULL REFERENCES empleado(id),
    motivo           text NOT NULL CHECK (motivo IN ('DESPIDO_RESPONSABILIDAD', 'RENUNCIA', 'FIN_CONTRATO', 'MUTUO_ACUERDO')),
    fecha_salida     date NOT NULL,
    estado           text NOT NULL DEFAULT 'BORRADOR' CHECK (estado IN ('BORRADOR', 'APROBADO', 'PAGADO', 'ANULADO')),
    -- Entradas capturadas (el saldo de vacaciones llega en Etapa 4; hoy lo indica RRHH).
    dias_vacaciones  numeric(6, 2) NOT NULL DEFAULT 0 CHECK (dias_vacaciones >= 0),
    -- Snapshot del cálculo (auditable e inmutable tras aprobar).
    salario_promedio numeric(14, 2) NOT NULL DEFAULT 0,
    salario_diario   numeric(14, 2) NOT NULL DEFAULT 0,
    anios_servicio   int NOT NULL DEFAULT 0,
    preaviso         numeric(14, 2) NOT NULL DEFAULT 0,
    cesantia         numeric(14, 2) NOT NULL DEFAULT 0,
    vacaciones       numeric(14, 2) NOT NULL DEFAULT 0,
    aguinaldo        numeric(14, 2) NOT NULL DEFAULT 0,
    descuentos       numeric(14, 2) NOT NULL DEFAULT 0, -- adelanto pendiente + saldos de préstamos
    total            numeric(14, 2) NOT NULL DEFAULT 0,
    detalle          jsonb NOT NULL DEFAULT '[]',
    creado_por       uuid,
    aprobado_por     uuid,
    pagado_por       uuid,
    creado_en        timestamptz NOT NULL DEFAULT now(),
    aprobado_en      timestamptz,
    pagado_en        timestamptz
);
-- Un finiquito vivo por empleado (un ANULADO permite rehacerlo).
CREATE UNIQUE INDEX uniq_finiquito_vivo ON finiquito (empresa_id, empleado_id) WHERE estado <> 'ANULADO';

-- Bitácora del archivo de pago SINPE: consecutivo por empresa + totales para auditoría.
CREATE TABLE nomina_archivo_pago (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id),
    corrida_id  uuid NOT NULL REFERENCES corrida_nomina(id),
    consecutivo int NOT NULL,
    registros   int NOT NULL,
    total       numeric(14, 2) NOT NULL,
    creado_por  uuid,
    creado_en   timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, consecutivo)
);
