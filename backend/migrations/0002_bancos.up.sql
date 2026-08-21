-- Bancos, cuentas, motor de tipo de cambio e importaciones.

CREATE TABLE banco (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre     text NOT NULL,
    activo     boolean NOT NULL DEFAULT true,
    creado_en  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_banco_empresa ON banco (empresa_id);

CREATE TABLE cuenta_bancaria (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    banco_id   uuid NOT NULL REFERENCES banco(id),
    iban       text,
    moneda     text NOT NULL CHECK (moneda IN ('CRC','USD')),
    alias      text,
    activo     boolean NOT NULL DEFAULT true,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    -- Memoria IBAN: un IBAN identifica una única cuenta por empresa (RN-02).
    UNIQUE (empresa_id, iban)
);
CREATE INDEX idx_cuenta_empresa ON cuenta_bancaria (empresa_id);
CREATE INDEX idx_cuenta_banco ON cuenta_bancaria (banco_id);

-- Cotizaciones puntuales (día 1 / 15 / último) — RN-10.
CREATE TABLE tipo_cambio_cotizacion (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    fecha      date NOT NULL,
    valor      numeric(14,4) NOT NULL,
    fuente     text NOT NULL CHECK (fuente IN ('BCCR','MANUAL')),
    creado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, fecha)
);
CREATE INDEX idx_tc_cot_empresa ON tipo_cambio_cotizacion (empresa_id, fecha);

-- TC mensual con congelamiento inmutable (RN-12/13).
CREATE TABLE tipo_cambio_mes (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    anio            int NOT NULL,
    mes             int NOT NULL CHECK (mes BETWEEN 1 AND 12),
    valor_congelado numeric(14,4),
    estado          text NOT NULL DEFAULT 'PROVISIONAL' CHECK (estado IN ('PROVISIONAL','CONGELADO')),
    congelado_en    timestamptz,
    creado_en       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, anio, mes)
);

CREATE TABLE importacion (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id         uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    cuenta_bancaria_id uuid NOT NULL REFERENCES cuenta_bancaria(id),
    source_file_hash   text NOT NULL,
    nombre_archivo     text NOT NULL,
    estado             text NOT NULL DEFAULT 'CARGADA' CHECK (estado IN ('CARGADA','PREVISUALIZADA','CONFIRMADA','CERRADA')),
    creado_por         uuid REFERENCES usuario(id),
    creado_en          timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_importacion_empresa ON importacion (empresa_id);
CREATE INDEX idx_importacion_cuenta ON importacion (cuenta_bancaria_id);
