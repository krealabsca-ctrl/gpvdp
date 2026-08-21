-- Fase 2 (CxP): catálogo de DEPARTAMENTOS (centros de costo) por empresa.
-- Segmento del gasto adicional al Concepto/Clasificación: ordena la factura por área
-- (Logística, Mercadeo, Ventas, Cobros, Finanzas…). Base del enrutamiento y de la
-- futura validación por departamento. Administrable desde la UI (crear/editar/desactivar).
-- Multi-tenant: todo aislado por empresa_id. Baja lógica (activo), nunca borrado físico.

CREATE TABLE departamento (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre         text NOT NULL,
    codigo         text,
    centro_costo   text,
    activo         boolean NOT NULL DEFAULT true,
    orden          int NOT NULL DEFAULT 0,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    -- Nombre único por empresa (permite crear el mismo departamento en otra empresa).
    UNIQUE (empresa_id, nombre)
);
CREATE INDEX idx_departamento_empresa ON departamento (empresa_id);
