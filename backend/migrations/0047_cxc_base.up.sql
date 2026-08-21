-- ============================================================================
-- CUENTAS POR COBRAR · Fase 1 — contratos y CARGOS (partidas abiertas)
-- ----------------------------------------------------------------------------
-- Decisiones del Director Financiero (2026-08-04):
--   1. CARGOS POR PERÍODO (partidas abiertas). Cada ciclo genera un cargo con su
--      vencimiento y su saldo; los cobros se APLICAN contra cargos. Así el saldo,
--      la antigüedad real y las modalidades no mensuales dejan de ser casos
--      especiales: son consecuencia del modelo.
--   2. NO se factura electrónicamente: el cargo es documento INTERNO de cobro.
--      Se dejó `clave_hacienda` para poder volverlo fiscal después sin rehacer.
--   3. Aplicación de cobros: el cargo MÁS VIEJO primero (fase 2).
--   4. El CONTRATO es el eje; el cliente es dato del contrato, SIN unicidad de
--      cédula (hay duplicados y variantes históricas). Se puede ascender el
--      cliente a entidad propia después sin tocar esto.
--
-- Multi-tenant: TODA tabla lleva empresa_id. Nunca borrado físico.
-- ============================================================================

-- btree_gist permite meter la igualdad de un uuid dentro de un índice gist, que es lo
-- que necesita el EXCLUDE de los tramos para ser «no se traslapan DENTRO de la misma
-- empresa» en vez de «no se traslapan en todo el sistema».
CREATE EXTENSION IF NOT EXISTS btree_gist;

-- ── Sede operativa ──────────────────────────────────────────────────────────
-- NO viene en los archivos del sistema de origen (verificado con los datasets
-- reales): es la asignación de cartera y se administra acá. Es la frontera de
-- datos del operador de cobros. Distinta de `departamento`, que en CxP son las
-- áreas que validan facturas.
CREATE TABLE cxc_sede (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre         text NOT NULL,
    -- Razón social y plaza tal como vienen en el campo «Sede» del archivo, que
    -- las trae pegadas: «SAN JOSÉ - VALLE DE PAZ DE COSTA RICA SA».
    razon_social   text,
    plaza          text,
    activa         boolean NOT NULL DEFAULT true,
    orden          int NOT NULL DEFAULT 0,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);
CREATE INDEX idx_cxc_sede_empresa ON cxc_sede (empresa_id);

-- ── Asociación solidarista ──────────────────────────────────────────────────
-- El canal DOMINANTE de cobro (11 de 11 pagos de la muestra real). La asociación
-- deduce de la planilla del trabajador, manda UN depósito y un detalle con
-- cientos de contratos. La mora de un contrato de este canal puede no ser culpa
-- del cliente sino de la asociación que no envió planilla.
CREATE TABLE cxc_asociacion (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre         text NOT NULL,
    -- Patrono/institución de donde sale la planilla (PINDECO, ICE…): la misma
    -- asociación puede tener varias.
    patrono        text,
    contacto       text,
    correo         text,
    activa         boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);
CREATE INDEX idx_cxc_asociacion_empresa ON cxc_asociacion (empresa_id);

-- ── Modalidad de cobro ──────────────────────────────────────────────────────
-- `meses_ciclo` es lo que usa el generador de cargos, y con eso las modalidades
-- no mensuales dejan de ser un problema de la meta del mes. La quincenal genera
-- DOS cargos por mes, así que se marca aparte en vez de forzar medio mes.
CREATE TABLE cxc_modalidad (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre         text NOT NULL,
    meses_ciclo    smallint NOT NULL DEFAULT 1 CHECK (meses_ciclo BETWEEN 1 AND 12),
    -- true = dos cargos por mes (1Q y 2Q), como aparece en los pagos reales.
    quincenal      boolean NOT NULL DEFAULT false,
    activa         boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);
CREATE INDEX idx_cxc_modalidad_empresa ON cxc_modalidad (empresa_id);

-- ── Forma de pago ───────────────────────────────────────────────────────────
-- `factor_recuperacion` multiplica la probabilidad del tramo para priorizar la
-- cola: el mismo saldo no vale igual si está domiciliado que si hay que ir a
-- buscarlo. Valores validados en el portal de Apps Script.
CREATE TABLE cxc_forma_pago (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre         text NOT NULL,
    factor_recuperacion numeric(4, 2) NOT NULL DEFAULT 1.00
        CHECK (factor_recuperacion BETWEEN 0.10 AND 2.00),
    -- true = el cobro llega por planilla de asociación, no del cliente.
    es_asociacion  boolean NOT NULL DEFAULT false,
    -- true = domiciliado (tarjeta): habilita la alerta de tarjeta vencida.
    es_domiciliado boolean NOT NULL DEFAULT false,
    activa         boolean NOT NULL DEFAULT true,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);
CREATE INDEX idx_cxc_forma_pago_empresa ON cxc_forma_pago (empresa_id);

-- ── Tramo de mora ───────────────────────────────────────────────────────────
-- La ventaja competitiva del prototipo: priorizar por VALOR ESPERADO
-- (saldo × probabilidad × factor de forma de pago) en vez de por antigüedad.
-- ADELANTADO existe porque los datos reales traen días vencidos NEGATIVOS
-- (−11, −26, −30): hay clientes que pagan antes.
CREATE TABLE cxc_tramo (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    codigo         text NOT NULL,
    etiqueta       text NOT NULL,
    dias_min       int NOT NULL,
    dias_max       int NOT NULL,
    orden          smallint NOT NULL,
    prob_recuperacion numeric(4, 2) NOT NULL
        CHECK (prob_recuperacion BETWEEN 0 AND 1),
    estrategia     text NOT NULL DEFAULT '',
    canal_sugerido text NOT NULL DEFAULT '',
    creado_en      timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, codigo),
    UNIQUE (empresa_id, orden),
    CHECK (dias_min <= dias_max),
    -- Dos tramos no pueden traslaparse: lo impide la BASE, no el código. Si se
    -- traslaparan, un mismo saldo tendría dos probabilidades y la cola sería
    -- irreproducible.
    EXCLUDE USING gist (
        empresa_id WITH =,
        int4range(dias_min, dias_max, '[]') WITH &&
    )
);

-- ── Contrato ────────────────────────────────────────────────────────────────
CREATE TABLE contrato_cxc (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    -- TEXTO, no entero: los datos reales traen «CO198456» y «CD-0000000561».
    numero         text NOT NULL,
    sede_id        uuid REFERENCES cxc_sede(id),
    -- El cliente es DATO del contrato (decisión 4). Sin UNIQUE en documento: hay
    -- duplicados y variantes históricas (con guiones, DIMEX, extranjeros).
    cliente_nombre text NOT NULL DEFAULT '',
    documento      text NOT NULL DEFAULT '',
    telefonos      text NOT NULL DEFAULT '',
    correos        text NOT NULL DEFAULT '',
    -- Servicio y comercial
    servicio       text NOT NULL DEFAULT '',
    tipo_servicio  text NOT NULL DEFAULT '',
    modalidad_id   uuid REFERENCES cxc_modalidad(id),
    forma_pago_id  uuid REFERENCES cxc_forma_pago(id),
    asociacion_id  uuid REFERENCES cxc_asociacion(id),
    -- Día del mes en que vence la cuota (columna «Dias de Pagos»).
    dia_pago       smallint CHECK (dia_pago IS NULL OR dia_pago BETWEEN 1 AND 31),
    cuota_vigente  numeric(14, 2) NOT NULL DEFAULT 0 CHECK (cuota_vigente >= 0),
    fecha_inicial  date,
    -- Desde acá genera cargos el motor. Es el dato clave de la fase 1.
    fecha_primer_cobro date,
    tarjeta_vence  date,
    estado         text NOT NULL DEFAULT 'ACTIVO'
        CHECK (estado IN ('ACTIVO', 'SUSPENDIDO', 'LEGAL', 'CANCELADO')),
    -- Datos del sistema de origen: se guardan para poder comparar contra él
    -- durante la corrida en paralelo, pero el ERP calcula su propio tramo con la
    -- antigüedad real de los cargos. NO se usan como verdad.
    score_origen         int,
    estado_origen        text NOT NULL DEFAULT '',
    morosidad_origen     text NOT NULL DEFAULT '',
    dias_vencidos_origen int,
    saldo_origen         numeric(14, 2),
    -- Cuarentena: la fila entró pero tiene un dato fuera de rango. Queda visible
    -- y marcada, fuera de la cola y de la meta, hasta que alguien la corrija.
    -- Nada entra en silencio y nada se pierde.
    revision_pendiente boolean NOT NULL DEFAULT false,
    revision_motivo    text NOT NULL DEFAULT '',
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, numero)
);
CREATE INDEX idx_contrato_cxc_empresa ON contrato_cxc (empresa_id);
CREATE INDEX idx_contrato_cxc_sede ON contrato_cxc (empresa_id, sede_id);
CREATE INDEX idx_contrato_cxc_documento ON contrato_cxc (empresa_id, documento);
CREATE INDEX idx_contrato_cxc_asociacion ON contrato_cxc (empresa_id, asociacion_id);
-- Búsqueda por nombre del cliente sobre 70 000 contratos: trigram, porque la
-- gente busca «vargas mora» y no el prefijo exacto.
CREATE EXTENSION IF NOT EXISTS pg_trgm;
CREATE INDEX idx_contrato_cxc_nombre_trgm ON contrato_cxc USING gin (cliente_nombre gin_trgm_ops);

-- ── Cargo: LA PARTIDA ABIERTA ───────────────────────────────────────────────
-- El corazón del módulo y lo que el sistema de origen no tiene. Cada cargo
-- guarda el monto que regía en SU período: un cambio de cuota no reescribe el
-- pasado.
--
-- `periodo` es texto con formato fijo: '2026-07' (mensual y multi-mes) o
-- '2026-07-1Q' / '2026-07-2Q' (quincenal). Es el mismo lenguaje que ya usa el
-- sistema de origen en el campo Concepto de sus pagos («M/JULIO», «1Q/JULIO»),
-- así que la migración puede casar unos con otros.
CREATE TABLE cargo_cxc (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    contrato_id    uuid NOT NULL REFERENCES contrato_cxc(id) ON DELETE CASCADE,
    periodo        text NOT NULL,
    vence_en       date NOT NULL,
    monto          numeric(14, 2) NOT NULL CHECK (monto > 0),
    -- Lo aplicado por cobros. El SALDO no se guarda: se deriva (monto − aplicado)
    -- para que no pueda quedar desincronizado, igual que en el resto del ERP.
    monto_aplicado numeric(14, 2) NOT NULL DEFAULT 0 CHECK (monto_aplicado >= 0),
    estado         text NOT NULL DEFAULT 'ABIERTO'
        CHECK (estado IN ('ABIERTO', 'PARCIAL', 'SALDADO', 'ANULADO')),
    origen         text NOT NULL DEFAULT 'GENERADO'
        CHECK (origen IN ('GENERADO', 'RECONSTRUIDO', 'SALDO_INICIAL', 'AJUSTE', 'IMPORTADO')),
    -- Hueco para el día en que estos cargos deban ser comprobantes electrónicos.
    -- Hoy NO se factura (decisión 2) y queda nulo.
    clave_hacienda text,
    nota           text NOT NULL DEFAULT '',
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    -- Generar dos veces el mismo período NO duplica: el generador es idempotente
    -- por construcción, no por cuidado del programador.
    UNIQUE (contrato_id, periodo),
    -- No se puede aplicar más de lo que vale el cargo. Un sobrepago va a saldo a
    -- favor del cliente, no a un cargo con saldo negativo.
    CHECK (monto_aplicado <= monto)
);
CREATE INDEX idx_cargo_cxc_empresa ON cargo_cxc (empresa_id);
CREATE INDEX idx_cargo_cxc_contrato ON cargo_cxc (contrato_id, vence_en);
-- El índice que sostiene la cola y el aging: los cargos que TODAVÍA se deben,
-- por fecha de vencimiento. Parcial, para que no cargue con los ya saldados
-- (que son la mayoría con el tiempo).
CREATE INDEX idx_cargo_cxc_abierto ON cargo_cxc (empresa_id, vence_en)
    WHERE estado IN ('ABIERTO', 'PARCIAL');

-- ── Parámetros del módulo ───────────────────────────────────────────────────
-- Los CONFIG del portal, ahora por empresa y con rastro de quién los cambió.
CREATE TABLE cxc_parametro (
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    clave          text NOT NULL,
    valor          text NOT NULL,
    descripcion    text NOT NULL DEFAULT '',
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    actualizado_por uuid REFERENCES usuario(id),
    PRIMARY KEY (empresa_id, clave)
);

-- ── Importación ─────────────────────────────────────────────────────────────
-- Cabecera de cada corrida del importador, con su reporte de conciliación. Se
-- persiste porque la pregunta «¿qué entró el 4 de agosto y qué quedó afuera?»
-- se hace semanas después.
CREATE TABLE cxc_importacion (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    tipo           text NOT NULL CHECK (tipo IN ('CONTRATOS', 'COBROS', 'PLANILLA')),
    archivo        text NOT NULL DEFAULT '',
    estado         text NOT NULL DEFAULT 'PREVISUALIZADA'
        CHECK (estado IN ('PREVISUALIZADA', 'CONFIRMADA', 'DESCARTADA')),
    filas          int NOT NULL DEFAULT 0,
    nuevos         int NOT NULL DEFAULT 0,
    actualizados   int NOT NULL DEFAULT 0,
    duplicados     int NOT NULL DEFAULT 0,
    cuarentena     int NOT NULL DEFAULT 0,
    reporte        jsonb,
    creado_por     uuid REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now(),
    confirmado_en  timestamptz
);
CREATE INDEX idx_cxc_importacion_empresa ON cxc_importacion (empresa_id, creado_en DESC);

-- ============================================================================
-- SEMILLA por empresa: catálogos con los valores validados en el portal.
-- Se siembra para las empresas existentes; una empresa nueva los recibe al
-- crearse (o con «aplicar faltantes»).
-- ============================================================================

INSERT INTO cxc_modalidad (empresa_id, nombre, meses_ciclo, quincenal)
SELECT e.id, m.nombre, m.ciclo, m.q
FROM empresa e
CROSS JOIN (VALUES
    ('Mensual', 1::smallint, false),
    ('Quincenal', 1::smallint, true),
    ('Trimestral', 3::smallint, false),
    ('Semestral', 6::smallint, false),
    ('Anual', 12::smallint, false)
) AS m(nombre, ciclo, q)
ON CONFLICT DO NOTHING;

-- Los factores vienen del portal. «Descuento por Asociación Solidarista» es el
-- canal dominante de los datos reales y NO estaba en ese catálogo: se siembra
-- en 1.00 (neutro) porque su factor real está PENDIENTE de definir con el
-- usuario; inventarlo distorsionaría el orden de la cola.
INSERT INTO cxc_forma_pago (empresa_id, nombre, factor_recuperacion, es_asociacion, es_domiciliado)
SELECT e.id, f.nombre, f.factor, f.asoc, f.dom
FROM empresa e
CROSS JOIN (VALUES
    ('Débito Automático', 1.15, false, true),
    ('Descuento por Asociación Solidarista', 1.00, true, false),
    ('Depósito o Transferencia', 1.00, false, false),
    ('Pago en Oficina', 0.90, false, false),
    ('Cobrador', 0.80, false, false)
) AS f(nombre, factor, asoc, dom)
ON CONFLICT DO NOTHING;

INSERT INTO cxc_tramo (empresa_id, codigo, etiqueta, dias_min, dias_max, orden, prob_recuperacion, estrategia, canal_sugerido)
SELECT e.id, t.codigo, t.etiqueta, t.dmin, t.dmax, t.orden, t.prob, t.estrategia, t.canal
FROM empresa e
CROSS JOIN (VALUES
    ('ADELANTADO', 'Adelantado',        -99999, -1,     1::smallint, 1.00, 'Fidelización o venta cruzada',      'Ninguno'),
    ('AL_DIA',     'Al día',                 0,  0,     2::smallint, 1.00, 'Confirmar domiciliación vigente',   'Ninguno'),
    ('PREVENTIVO', 'Preventivo 1-15',        1, 15,     3::smallint, 0.90, 'Recordatorio automático',           'WhatsApp o SMS'),
    ('TEMPRANO',   'Temprano 16-30',        16, 30,     4::smallint, 0.75, 'Llamada del operador; fijar fecha', 'Llamada'),
    ('MEDIO',      'Intensivo 31-60',       31, 60,     5::smallint, 0.55, 'Arreglo de pago escrito',           'Llamada + correo'),
    ('TARDIO',     'Prejurídico 61-90',     61, 90,     6::smallint, 0.35, 'Notificación formal',               'Carta y llamada'),
    ('CRITICO',    'Crítico 91-180',        91, 180,    7::smallint, 0.15, 'Última oportunidad',                'Visita o carta'),
    ('LEGAL',      'Legal +180',           181, 999999, 8::smallint, 0.05, 'Cobro judicial o castigo',          'Cobro judicial')
) AS t(codigo, etiqueta, dmin, dmax, orden, prob, estrategia, canal)
ON CONFLICT DO NOTHING;

INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, p.clave, p.valor, p.descripcion
FROM empresa e
CROSS JOIN (VALUES
    ('CUOTA_MAXIMA_RAZONABLE', '500000', 'Sobre esto la fila va a cuarentena: probable saldo pegado en el campo de cuota'),
    ('COBRO_MAXIMO_RAZONABLE', '1000000', 'Sobre esto el cobro se marca para revisión'),
    ('DIAS_ALERTA_TARJETA', '60', 'Aviso previo de vencimiento de tarjeta'),
    ('DIAS_PROMESA_VIGENTE', '15', 'Vigencia de una promesa de pago'),
    ('APLICACION_COBROS', 'MAS_VIEJO', 'Orden de aplicación de un cobro entre los cargos abiertos'),
    ('CARGOS_DESDE', '', 'Fecha mínima para generar cargos históricos (vacío = desde el primer cobro de cada contrato)')
) AS p(clave, valor, descripcion)
ON CONFLICT DO NOTHING;
