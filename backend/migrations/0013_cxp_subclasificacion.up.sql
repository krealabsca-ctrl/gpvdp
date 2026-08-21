-- CxP: 3er nivel de gasto (Subclasificación / Departamento) colgando de una Clasificación.
-- Ej.: Concepto "Gastos" › Clasificación "Combustible" › Subclasificación "Operaciones".
-- Es exclusivo de CxP; el módulo Bancos sigue usando 2 niveles (Concepto › Clasificación).

CREATE TABLE subclasificacion (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    clasificacion_id uuid NOT NULL REFERENCES clasificacion(id) ON DELETE CASCADE,
    nombre           text NOT NULL,
    activo           boolean NOT NULL DEFAULT true,
    creado_en        timestamptz NOT NULL DEFAULT now(),
    UNIQUE (clasificacion_id, nombre)
);
CREATE INDEX idx_subclasif_empresa ON subclasificacion (empresa_id);
CREATE INDEX idx_subclasif_clasif ON subclasificacion (clasificacion_id);

ALTER TABLE documento_cxp ADD COLUMN subclasificacion_id uuid REFERENCES subclasificacion(id);
CREATE INDEX idx_docxp_subclasif ON documento_cxp (subclasificacion_id);
