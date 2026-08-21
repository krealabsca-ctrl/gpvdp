-- Fase 2 (CxP): maestro de proveedores. Retención en la fuente parametrizable por proveedor
-- (cr-fiscal-compliance). El flujo de documentos/aprobaciones se agrega cuando se definan las reglas.

CREATE TABLE proveedor (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id          uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre              text NOT NULL,
    tipo_identificacion text CHECK (tipo_identificacion IN ('FISICA', 'JURIDICA', 'DIMEX', 'NITE')),
    identificacion      text,
    email               text,
    telefono            text,
    iban                text,
    retencion_renta_pct numeric(5, 2) NOT NULL DEFAULT 0,
    exento_iva          boolean NOT NULL DEFAULT false,
    activo              boolean NOT NULL DEFAULT true,
    creado_en           timestamptz NOT NULL DEFAULT now(),
    actualizado_en      timestamptz NOT NULL DEFAULT now(),
    -- Identificación única por empresa (permite varios NULL para proveedores sin cédula).
    UNIQUE (empresa_id, identificacion)
);
CREATE INDEX idx_proveedor_empresa ON proveedor (empresa_id);
