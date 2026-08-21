-- CxP: Caja chica bajo el sistema de FONDO FIJO (imprest), maqueta aprobada 2026-07-27.
-- Cada caja es un fondo de monto fijo con custodio responsable. Los gastos menores se
-- registran como VALES contra el fondo (comprobante electrónico o recibo manual). Lo único
-- que viaja por CxP es la REPOSICIÓN: un documento tipo REINTEGRO pagadero al custodio
-- (proveedor interno) que agrupa los vales; al pagarse, el fondo se restaura.
--
-- El saldo del fondo y el estado del vale se DERIVAN (sin triggers):
--   vale cuenta contra el saldo mientras su reposición no esté PAGADA/CONCILIADA;
--   vale "repuesto" cuando su reposición se pagó. Nunca DELETE físico (anulado = soft).
CREATE TABLE caja_chica_fondo (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    nombre          text NOT NULL,
    custodio_id     uuid REFERENCES usuario(id),
    departamento_id uuid REFERENCES departamento(id),
    -- Proveedor interno del custodio: beneficiario de la reposición (decisión 5 de la maqueta).
    proveedor_id    uuid REFERENCES proveedor(id),
    monto_asignado  numeric(14, 2) NOT NULL CHECK (monto_asignado > 0),
    umbral_pct      numeric(5, 2) NOT NULL DEFAULT 30 CHECK (umbral_pct >= 0 AND umbral_pct <= 100),
    limite_vale     numeric(14, 2) NOT NULL DEFAULT 0 CHECK (limite_vale >= 0), -- 0 = sin límite
    activo          boolean NOT NULL DEFAULT true,
    creado_en       timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);

CREATE TABLE caja_chica_vale (
    id                  uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id          uuid NOT NULL REFERENCES empresa(id),
    fondo_id            uuid NOT NULL REFERENCES caja_chica_fondo(id),
    fecha               date NOT NULL DEFAULT CURRENT_DATE,
    detalle             text NOT NULL,
    monto_crc           numeric(14, 2) NOT NULL CHECK (monto_crc > 0),
    concepto_id         uuid REFERENCES concepto(id),
    clasificacion_id    uuid REFERENCES clasificacion(id),
    subclasificacion_id uuid REFERENCES subclasificacion(id),
    -- FE = factura electrónica (deducible) · RECIBO = recibo manual (no deducible).
    comprobante         text NOT NULL DEFAULT 'RECIBO' CHECK (comprobante IN ('FE', 'RECIBO')),
    registrado_por      uuid REFERENCES usuario(id),
    reposicion_id       uuid REFERENCES documento_cxp(id), -- NULL = pendiente de reponer
    anulado             boolean NOT NULL DEFAULT false,    -- soft (error de digitación); nunca DELETE
    creado_en           timestamptz NOT NULL DEFAULT now()
);

CREATE INDEX idx_caja_vale_fondo ON caja_chica_vale (fondo_id) WHERE NOT anulado;
CREATE INDEX idx_caja_vale_reposicion ON caja_chica_vale (reposicion_id) WHERE reposicion_id IS NOT NULL;
CREATE INDEX idx_caja_fondo_empresa ON caja_chica_fondo (empresa_id);
