-- Catálogos por empresa: Concepto, Clasificacion y motor de reglas.

CREATE TABLE concepto (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre     text NOT NULL,
    activo     boolean NOT NULL DEFAULT true,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);
CREATE INDEX idx_concepto_empresa ON concepto (empresa_id);

CREATE TABLE clasificacion (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id             uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    concepto_id            uuid NOT NULL REFERENCES concepto(id) ON DELETE CASCADE,
    nombre                 text NOT NULL,
    -- Preparación contable futura (mapeo NIIF sin migrar datos) — spec §25.
    cuenta_contable_futura text,
    activo                 boolean NOT NULL DEFAULT true,
    creado_en              timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, concepto_id, nombre),
    -- Respalda la FK compuesta (id, concepto_id) usada por regla_clasificacion y movimiento_bancario.
    UNIQUE (id, concepto_id)
);
CREATE INDEX idx_clasificacion_empresa ON clasificacion (empresa_id);
CREATE INDEX idx_clasificacion_concepto ON clasificacion (concepto_id);

CREATE TABLE regla_clasificacion (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre           text NOT NULL,
    aplica_a         text NOT NULL CHECK (aplica_a IN ('DEBITO','CREDITO','MIXTO')),
    concepto_id      uuid NOT NULL REFERENCES concepto(id),
    clasificacion_id uuid NOT NULL,
    prioridad        int NOT NULL DEFAULT 100,
    activo           boolean NOT NULL DEFAULT true,
    creado_en        timestamptz NOT NULL DEFAULT now(),
    -- Integridad: la clasificacion debe pertenecer al concepto indicado.
    FOREIGN KEY (clasificacion_id, concepto_id) REFERENCES clasificacion (id, concepto_id)
);
CREATE INDEX idx_regla_empresa ON regla_clasificacion (empresa_id);

CREATE TABLE palabra_clave (
    id        uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    regla_id  uuid NOT NULL REFERENCES regla_clasificacion(id) ON DELETE CASCADE,
    texto     text NOT NULL,
    creado_en timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_palabra_regla ON palabra_clave (regla_id);
