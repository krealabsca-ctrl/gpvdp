-- ============================================================================
-- CUENTAS POR COBRAR · Fase 2 — COBROS y su APLICACIÓN contra los cargos
-- ----------------------------------------------------------------------------
-- Decisión del Director Financiero: los cobros se aplican al cargo MÁS VIEJO
-- primero (FIFO), con aplicación manual disponible por excepción.
--
-- Lo que los datos reales exigieron y no estaba en el plan original:
--   · El canal dominante es el DESCUENTO POR ASOCIACIÓN SOLIDARISTA (11 de 11
--     pagos de la muestra). La asociación deduce de planilla, manda UN depósito
--     y un detalle con cientos de contratos ⇒ tabla `cxc_planilla`, y la
--     conciliación es del LOTE (esperado vs depositado), no de cada pago.
--   · Cada pago trae TRES fechas con significados distintos: la del período,
--     la bancaria (cuándo entró la plata) y la de registro. Se guardan las tres
--     porque cada una responde una pregunta diferente.
--   · Un mismo cobro puede pagar DOS períodos, uno de ellos parcial. La
--     aplicación múltiple es la operación normal, no una excepción.
-- ============================================================================

-- ── Planilla de asociación: el lote del canal dominante ─────────────────────
CREATE TABLE cxc_planilla (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    asociacion_id  uuid NOT NULL REFERENCES cxc_asociacion(id),
    -- Referencia del lote tal como la nombra la asociación: «ADEPSA-PINDECO-IQ-JULIO-2026».
    referencia     text NOT NULL,
    periodo        text NOT NULL DEFAULT '',
    -- Esperado: lo que debería venir según los contratos de esa asociación.
    -- Depositado: lo que el banco recibió. La DIFERENCIA es el hallazgo.
    esperado       numeric(16, 2) NOT NULL DEFAULT 0,
    depositado     numeric(16, 2) NOT NULL DEFAULT 0,
    -- La plata de una planilla puede llegar en VARIAS transferencias: el dato real
    -- traía «08/07/2026|11/07/2026» en un mismo registro. Por eso es un arreglo.
    fechas_bancarias date[] NOT NULL DEFAULT '{}',
    estado         text NOT NULL DEFAULT 'PENDIENTE'
        CHECK (estado IN ('PENDIENTE', 'RECIBIDA', 'CONCILIADA', 'CON_DIFERENCIA')),
    nota           text NOT NULL DEFAULT '',
    creado_por     uuid REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, asociacion_id, referencia)
);
CREATE INDEX idx_cxc_planilla_empresa ON cxc_planilla (empresa_id, estado);

-- ── Cobro: el dinero que entró ──────────────────────────────────────────────
CREATE TABLE cobro_cxc (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    -- NULL a propósito: un depósito que todavía no se sabe de quién es entra igual y
    -- espera en la bandeja de «sin identificar». No se descarta ni se adivina.
    contrato_id    uuid REFERENCES contrato_cxc(id),
    consecutivo    text NOT NULL DEFAULT '',
    -- Las TRES fechas del dato real:
    --   fecha_pago      = el período que se está pagando (el origen pone el día 1)
    --   fecha_bancaria  = cuándo entró la plata (la que concilia contra Bancos)
    --   fecha_registro  = cuándo se registró
    fecha_pago     date NOT NULL,
    fecha_bancaria date,
    fecha_registro date,
    monto          numeric(16, 2) NOT NULL CHECK (monto > 0),
    -- Remanente que no calzó en ningún cargo: queda a favor del cliente.
    -- No se guarda el saldo a favor «del cliente» como total: se DERIVA sumando esto.
    saldo_a_favor  numeric(16, 2) NOT NULL DEFAULT 0 CHECK (saldo_a_favor >= 0),
    forma_pago_id  uuid REFERENCES cxc_forma_pago(id),
    asociacion_id  uuid REFERENCES cxc_asociacion(id),
    planilla_id    uuid REFERENCES cxc_planilla(id),
    referencia     text NOT NULL DEFAULT '',
    -- Concepto crudo del sistema de origen. Se guarda tal cual porque de ahí se leyó
    -- el período aplicado y hay que poder auditar la interpretación.
    concepto_origen text NOT NULL DEFAULT '',
    origen         text NOT NULL DEFAULT 'ARCHIVO'
        CHECK (origen IN ('ARCHIVO', 'API', 'CAJA', 'BANCO', 'PLANILLA')),
    estado         text NOT NULL DEFAULT 'APLICADO'
        CHECK (estado IN ('APLICADO', 'SIN_IDENTIFICAR', 'REVERSADO')),
    -- Idempotencia de verdad: reenviar el mismo cobro no lo duplica. Lo usa la API
    -- (cabecera Idempotency-Key) y el importador (consecutivo del archivo).
    idempotency_key text,
    -- Puente con Bancos: el movimiento del estado de cuenta que trajo esta plata.
    movimiento_bancario_id uuid REFERENCES movimiento_bancario(id),
    -- Reversa (cheque devuelto, débito rechazado, contracargo): no se borra el cobro,
    -- se marca y se desaplican sus cargos, que vuelven a abrirse con su antigüedad.
    reversado_por  uuid REFERENCES usuario(id),
    reversado_en   timestamptz,
    reversa_motivo text NOT NULL DEFAULT '',
    creado_por     uuid REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    -- Un cobro sin identificar no tiene contrato: por eso el índice es parcial.
    -- La regla del sistema viejo era «no dos veces el mismo recibo en el mismo
    -- contrato»; acá es física del esquema, no código defensivo.
    CONSTRAINT cobro_cxc_saldo_favor_coherente CHECK (saldo_a_favor <= monto)
);
CREATE UNIQUE INDEX idx_cobro_cxc_consecutivo ON cobro_cxc (empresa_id, contrato_id, consecutivo)
    WHERE contrato_id IS NOT NULL AND consecutivo <> '';
CREATE UNIQUE INDEX idx_cobro_cxc_idempotencia ON cobro_cxc (empresa_id, idempotency_key)
    WHERE idempotency_key IS NOT NULL AND idempotency_key <> '';
CREATE INDEX idx_cobro_cxc_empresa ON cobro_cxc (empresa_id, fecha_pago DESC);
CREATE INDEX idx_cobro_cxc_contrato ON cobro_cxc (contrato_id, fecha_pago DESC);
CREATE INDEX idx_cobro_cxc_planilla ON cobro_cxc (planilla_id);
-- La bandeja de «sin identificar» es una consulta caliente: índice parcial.
CREATE INDEX idx_cobro_cxc_sin_identificar ON cobro_cxc (empresa_id, fecha_bancaria)
    WHERE estado = 'SIN_IDENTIFICAR';

-- ── Aplicación: qué cobro pagó qué cargo ────────────────────────────────────
-- La tabla que hace que «cuánto se debe» y «cuánto se cobró» sean la misma verdad.
CREATE TABLE cobro_aplicacion (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    cobro_id    uuid NOT NULL REFERENCES cobro_cxc(id) ON DELETE CASCADE,
    cargo_id    uuid NOT NULL REFERENCES cargo_cxc(id) ON DELETE CASCADE,
    monto       numeric(16, 2) NOT NULL CHECK (monto > 0),
    -- true cuando el cargo quedó parcialmente pagado por esta aplicación.
    parcial     boolean NOT NULL DEFAULT false,
    creado_en   timestamptz NOT NULL DEFAULT now(),
    -- Un cobro no puede aplicarse dos veces al mismo cargo: si hay que corregir, se
    -- reversa el cobro y se vuelve a aplicar.
    UNIQUE (cobro_id, cargo_id)
);
CREATE INDEX idx_cobro_aplicacion_cargo ON cobro_aplicacion (cargo_id);
CREATE INDEX idx_cobro_aplicacion_empresa ON cobro_aplicacion (empresa_id);

-- ── Nota de crédito: condonar, descontar o corregir ─────────────────────────
-- NO edita el cargo original: emite un documento que lo reduce, con autorización y
-- rastro. Es la única forma legítima de bajar una deuda sin que entre plata.
CREATE TABLE nota_credito_cxc (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    contrato_id uuid NOT NULL REFERENCES contrato_cxc(id),
    cargo_id    uuid REFERENCES cargo_cxc(id),
    fecha       date NOT NULL,
    monto       numeric(16, 2) NOT NULL CHECK (monto > 0),
    motivo      text NOT NULL,
    estado      text NOT NULL DEFAULT 'APLICADA'
        CHECK (estado IN ('APLICADA', 'ANULADA')),
    creado_por  uuid REFERENCES usuario(id),
    creado_en   timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_nota_credito_cxc_contrato ON nota_credito_cxc (contrato_id, fecha DESC);

-- Parámetro nuevo: el tope de un cobro verosímil (el sistema viejo tuvo pagos con
-- cédulas pegadas como monto).
INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, 'FECHA_COBRO_DEL_MES', 'BANCARIA',
       'Cuál de las tres fechas cuenta para «cobrado en el mes»: PAGO · BANCARIA · REGISTRO'
FROM empresa e
ON CONFLICT DO NOTHING;
