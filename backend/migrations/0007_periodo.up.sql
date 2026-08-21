-- Cierre de período. Un registro = período cerrado (RN-22).
CREATE TABLE periodo_cierre (
    id                         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id                 uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    anio                       int NOT NULL,
    mes                        int NOT NULL CHECK (mes BETWEEN 1 AND 12),
    no_identificados_al_cierre int NOT NULL DEFAULT 0,
    cerrado_por                uuid REFERENCES usuario(id),
    cerrado_en                 timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, anio, mes)
);
CREATE INDEX idx_periodo_cierre_empresa ON periodo_cierre (empresa_id, anio, mes);
