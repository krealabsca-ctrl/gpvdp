-- Movimiento bancario: modelo canónico (spec §18).
-- NOTA Fase 1: particionar por (empresa_id, anio, mes) para el volumen proyectado.
-- En Fase 0 se deja como tabla simple con índices compuestos.

CREATE TABLE movimiento_bancario (
    id                   uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id           uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    cuenta_bancaria_id   uuid NOT NULL REFERENCES cuenta_bancaria(id),
    importacion_id       uuid REFERENCES importacion(id),
    fecha                date NOT NULL,
    documento            text,
    descripcion          text,
    debito               numeric(16,2) NOT NULL DEFAULT 0,
    credito              numeric(16,2) NOT NULL DEFAULT 0,
    moneda_original      text NOT NULL DEFAULT 'CRC' CHECK (moneda_original IN ('CRC','USD')),
    monto_original       numeric(16,2) NOT NULL DEFAULT 0,
    monto_crc            numeric(16,2) NOT NULL DEFAULT 0,
    tc_aplicado          numeric(14,4),
    concepto_id          uuid REFERENCES concepto(id),
    clasificacion_id     uuid,
    estado_clasificacion text NOT NULL DEFAULT 'NO_IDENTIFICADO'
        CHECK (estado_clasificacion IN ('NO_IDENTIFICADO','AUTO','REVISADO')),
    confianza            numeric(5,2),
    es_traslado          boolean NOT NULL DEFAULT false,
    par_traslado_id      uuid REFERENCES movimiento_bancario(id),
    natural_key          text NOT NULL,
    indice_ocurrencia    int NOT NULL DEFAULT 1,
    incluido             boolean NOT NULL DEFAULT true,
    origen_historico     boolean NOT NULL DEFAULT false,
    creado_en            timestamptz NOT NULL DEFAULT now(),
    actualizado_en       timestamptz NOT NULL DEFAULT now(),
    -- Idempotencia de reimportación (RN-08): clave natural única por empresa.
    UNIQUE (empresa_id, natural_key),
    -- Si se clasifica, la clasificacion debe pertenecer al concepto (se valida solo cuando ambos existen).
    FOREIGN KEY (clasificacion_id, concepto_id) REFERENCES clasificacion (id, concepto_id)
);
CREATE INDEX idx_mov_empresa_fecha  ON movimiento_bancario (empresa_id, fecha);
CREATE INDEX idx_mov_empresa_estado ON movimiento_bancario (empresa_id, estado_clasificacion);
CREATE INDEX idx_mov_cuenta         ON movimiento_bancario (cuenta_bancaria_id);
CREATE INDEX idx_mov_importacion    ON movimiento_bancario (importacion_id);
