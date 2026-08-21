-- Fase 2 (CxP): aplicación de anticipos a proveedores (netting).
-- Un anticipo es un documento_cxp con tipo='ANTICIPO' ya pagado; queda como saldo a favor
-- del proveedor. Cuando llega la factura final, Contabilidad aplica uno o varios anticipos y
-- el neto a pagar = total_crc de la factura − suma de aplicaciones activas.
--
-- La aplicación es reversible mientras la factura no esté pagada (activo=false + huella de reverso).
-- v1: neteo en colones (monto_crc); la aplicación exige que anticipo y factura sean CRC.
CREATE TABLE anticipo_aplicacion (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id),
    anticipo_id   uuid NOT NULL REFERENCES documento_cxp(id),   -- documento tipo ANTICIPO
    factura_id    uuid NOT NULL REFERENCES documento_cxp(id),   -- factura que recibe el crédito
    monto_crc     numeric(14, 2) NOT NULL CHECK (monto_crc > 0),
    aplicado_por  uuid REFERENCES usuario(id),
    aplicado_en   timestamptz NOT NULL DEFAULT now(),
    activo        boolean NOT NULL DEFAULT true,                -- soft-reverse (nunca DELETE físico)
    reversado_por uuid REFERENCES usuario(id),
    reversado_en  timestamptz,
    CHECK (anticipo_id <> factura_id)
);

-- Saldo del anticipo y neto de la factura se calculan sumando aplicaciones activas.
CREATE INDEX idx_anticipo_aplicacion_anticipo ON anticipo_aplicacion (anticipo_id) WHERE activo;
CREATE INDEX idx_anticipo_aplicacion_factura  ON anticipo_aplicacion (factura_id)  WHERE activo;
CREATE INDEX idx_anticipo_aplicacion_empresa  ON anticipo_aplicacion (empresa_id);
