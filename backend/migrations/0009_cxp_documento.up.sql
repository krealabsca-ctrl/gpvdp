-- Fase 2 (CxP): documento (factura de proveedor) + aprobaciones.
-- Flujo: RECIBIDO → REVISADO → APROBADO → PROGRAMADO → PAGADO → CONCILIADO.

CREATE TABLE documento_cxp (
    id                    uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id            uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    proveedor_id          uuid NOT NULL REFERENCES proveedor(id),
    clave                 text NOT NULL,
    consecutivo           text,
    fecha_emision         date NOT NULL,
    moneda                text NOT NULL DEFAULT 'CRC' CHECK (moneda IN ('CRC', 'USD')),
    subtotal              numeric(16, 2) NOT NULL DEFAULT 0,
    iva                   numeric(16, 2) NOT NULL DEFAULT 0,
    retencion             numeric(16, 2) NOT NULL DEFAULT 0,
    total                 numeric(16, 2) NOT NULL DEFAULT 0,
    tc_aplicado           numeric(14, 4),
    total_crc             numeric(16, 2) NOT NULL DEFAULT 0,
    descripcion           text,
    estado                text NOT NULL DEFAULT 'RECIBIDO'
        CHECK (estado IN ('RECIBIDO', 'REVISADO', 'APROBADO', 'PROGRAMADO', 'PAGADO', 'CONCILIADO')),
    fecha_pago_programada date,
    huella                text,
    creado_por            uuid REFERENCES usuario(id),
    creado_en             timestamptz NOT NULL DEFAULT now(),
    actualizado_en        timestamptz NOT NULL DEFAULT now(),
    -- Clave de 50 dígitos = llave anti-duplicado del comprobante (Hacienda 4.4).
    UNIQUE (empresa_id, clave)
);
CREATE INDEX idx_docxp_empresa_estado ON documento_cxp (empresa_id, estado);
CREATE INDEX idx_docxp_proveedor ON documento_cxp (proveedor_id);

CREATE TABLE documento_cxp_aprobacion (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id   uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    documento_id uuid NOT NULL REFERENCES documento_cxp(id) ON DELETE CASCADE,
    usuario_id   uuid NOT NULL REFERENCES usuario(id),
    rol          text NOT NULL,
    aprobado_en  timestamptz NOT NULL DEFAULT now(),
    -- Un usuario aprueba una sola vez cada documento (evita contar dos firmas de la misma persona).
    UNIQUE (documento_id, usuario_id)
);
CREATE INDEX idx_docxp_aprob_doc ON documento_cxp_aprobacion (documento_id);
