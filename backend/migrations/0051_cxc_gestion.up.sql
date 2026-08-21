-- ============================================================================
-- CUENTAS POR COBRAR · Fase 3 — GESTIÓN DE COBRO
-- ----------------------------------------------------------------------------
-- La cola de cobro se ordena por VALOR ESPERADO, no por antigüedad:
--
--     saldo × probabilidad del tramo × factor de la forma de pago
--
-- Es la ventaja real del prototipo de Apps Script y no la trae ningún ERP de
-- estante: pone primero los casos donde una llamada cambia el mes, en vez de
-- los más viejos (que suelen ser justo los menos recuperables).
--
-- La gestión se registra para que la cola sepa qué ya se trabajó. Sin eso, el
-- operador vuelve a llamar al mismo cliente tres veces y no llama a otros mil.
-- ============================================================================

-- ── Canal por el que se gestionó ────────────────────────────────────────────
CREATE TABLE cxc_canal_gestion (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre     text NOT NULL,
    orden      smallint NOT NULL DEFAULT 0,
    activo     boolean NOT NULL DEFAULT true,
    creado_en  timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);

-- ── Resultado de la gestión ─────────────────────────────────────────────────
-- Las dos banderas son las que permiten medir tres cosas distintas que el
-- prototipo ya separaba bien:
--   · contactabilidad: ¿llegamos al cliente?      → es_contacto
--   · negociación:     ¿se comprometió?            → exige_promesa
--   · cumplimiento:    ¿cumplió lo que prometió?   → derivado de los cobros
CREATE TABLE cxc_resultado_gestion (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    codigo        text NOT NULL,
    etiqueta      text NOT NULL,
    -- ¿La gestión llegó al cliente? Un «no contesta» no es contacto.
    es_contacto   boolean NOT NULL DEFAULT true,
    -- ¿Este resultado obliga a registrar una promesa con fecha y monto?
    exige_promesa boolean NOT NULL DEFAULT false,
    -- ¿Marca el dato de contacto como malo? (número equivocado, no existe)
    dato_malo     boolean NOT NULL DEFAULT false,
    orden         smallint NOT NULL DEFAULT 0,
    activo        boolean NOT NULL DEFAULT true,
    creado_en     timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, codigo)
);

-- ── Gestión: una llamada, un mensaje, una visita ────────────────────────────
CREATE TABLE gestion_cxc (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id   uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    contrato_id  uuid NOT NULL REFERENCES contrato_cxc(id) ON DELETE CASCADE,
    usuario_id   uuid REFERENCES usuario(id),
    canal_id     uuid NOT NULL REFERENCES cxc_canal_gestion(id),
    resultado_id uuid NOT NULL REFERENCES cxc_resultado_gestion(id),
    notas        text NOT NULL DEFAULT '',
    -- Foto del estado del contrato AL MOMENTO de gestionar. Se guarda porque la
    -- pregunta «¿cuánto debía cuando lo llamamos?» no se puede reconstruir después:
    -- el saldo de hoy ya cambió.
    saldo_al_gestionar numeric(14, 2) NOT NULL DEFAULT 0,
    dias_mora_al_gestionar int NOT NULL DEFAULT 0,
    tramo_al_gestionar text NOT NULL DEFAULT '',
    gestionada_en timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_gestion_cxc_contrato ON gestion_cxc (contrato_id, gestionada_en DESC);
CREATE INDEX idx_gestion_cxc_usuario ON gestion_cxc (empresa_id, usuario_id, gestionada_en DESC);
CREATE INDEX idx_gestion_cxc_fecha ON gestion_cxc (empresa_id, gestionada_en DESC);

-- ── Promesa de pago ─────────────────────────────────────────────────────────
-- Entidad propia y no un campo de la gestión: una promesa tiene vida después de
-- la llamada (se cumple o no) y su cumplimiento es una MÉTRICA, no una nota.
--
-- NO se guarda si se cumplió. El cumplimiento se DERIVA de los cobros: hay pago
-- del contrato entre el día de la promesa y la fecha prometida + su tolerancia,
-- por al menos el monto prometido. Guardarlo obligaría a un job que mantenga la
-- columna al día y a decidir cuál de las dos verdades gana cuando difieran: la
-- misma disciplina que el saldo, el tramo y el aging en el resto del ERP.
CREATE TABLE promesa_pago_cxc (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    gestion_id    uuid NOT NULL REFERENCES gestion_cxc(id) ON DELETE CASCADE,
    contrato_id   uuid NOT NULL REFERENCES contrato_cxc(id) ON DELETE CASCADE,
    fecha_promesa date NOT NULL,
    monto         numeric(14, 2) CHECK (monto IS NULL OR monto > 0),
    creado_en     timestamptz NOT NULL DEFAULT now()
);
CREATE INDEX idx_promesa_cxc_contrato ON promesa_pago_cxc (contrato_id, fecha_promesa DESC);
CREATE INDEX idx_promesa_cxc_fecha ON promesa_pago_cxc (empresa_id, fecha_promesa DESC);

-- ============================================================================
-- SEMILLA de los catálogos, con los valores validados en el portal.
-- ============================================================================

INSERT INTO cxc_canal_gestion (empresa_id, nombre, orden)
SELECT e.id, c.nombre, c.orden
FROM empresa e
CROSS JOIN (VALUES
    ('Llamada', 1::smallint), ('WhatsApp', 2::smallint), ('SMS', 3::smallint),
    ('Correo', 4::smallint), ('Visita', 5::smallint), ('Carta', 6::smallint)
) AS c(nombre, orden)
ON CONFLICT DO NOTHING;

INSERT INTO cxc_resultado_gestion (empresa_id, codigo, etiqueta, es_contacto, exige_promesa, dato_malo, orden)
SELECT e.id, r.codigo, r.etiqueta, r.contacto, r.promesa, r.malo, r.orden
FROM empresa e
CROSS JOIN (VALUES
    ('PROMESA_PAGO',    'Promesa de pago',            true,  true,  false, 1::smallint),
    ('ARREGLO',         'Solicita arreglo de pago',   true,  false, false, 2::smallint),
    ('CONTACTO_SIN_COMPROMISO', 'Contacto sin compromiso', true, false, false, 3::smallint),
    ('YA_PAGO',         'Dice que ya pagó',           true,  false, false, 4::smallint),
    ('SE_NIEGA',        'Se niega a pagar',           true,  false, false, 5::smallint),
    ('NO_CONTESTA',     'No contesta',                false, false, false, 6::smallint),
    ('BUZON',           'Buzón de voz',               false, false, false, 7::smallint),
    ('NUMERO_ERRADO',   'Número equivocado',          false, false, true,  8::smallint),
    ('SIN_DATOS',       'Sin datos de contacto',      false, false, true,  9::smallint)
) AS r(codigo, etiqueta, contacto, promesa, malo, orden)
ON CONFLICT DO NOTHING;

INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, p.clave, p.valor, p.descripcion
FROM empresa e
CROSS JOIN (VALUES
    ('DIAS_SIN_GESTIONAR', '30', 'Días sin gestión para marcar un contrato como desatendido en la cola'),
    ('PROMESA_TOLERANCIA_DIAS', '3', 'Días de gracia después de la fecha prometida antes de darla por incumplida')
) AS p(clave, valor, descripcion)
ON CONFLICT DO NOTHING;
