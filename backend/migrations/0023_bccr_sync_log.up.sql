-- Fase D: bitácora de sincronización con el BCCR (§22/§23). Cada intento (manual o
-- automático los días 1/15/último) queda registrado con su resultado, para mostrar
-- "última sincronización" y sustentar el fallback de §19 (usar último valor + bandera).
CREATE TABLE bccr_sync_log (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    fecha      date NOT NULL,            -- fecha objetivo de la cotización
    indicador  text NOT NULL,            -- p. ej. 318 (venta) / 317 (compra)
    valor      numeric(14,4),            -- NULL si el intento falló
    exito      boolean NOT NULL,
    mensaje    text,
    creado_en  timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_bccr_sync_empresa ON bccr_sync_log (empresa_id, creado_en DESC);
