-- 0116 · Presupuesto de INGRESO: la mitad de la meta que faltaba.
--
-- ── POR QUÉ NO ALCANZABA CON LO QUE HABÍA ───────────────────────────────────────────────────
--
-- El presupuesto existente es solo de GASTO, por departamento. Con media meta no hay resultado
-- presupuestado, ni margen, ni EBITDA presupuestado: se controla cuánto se gasta sin saber contra
-- qué. «Vamos bien» no se puede afirmar mirando un solo lado.
--
-- ── POR QUÉ UNA TABLA PROPIA Y NO LA DE GASTO ───────────────────────────────────────────────
--
-- Porque la dimensión es distinta y no se fuerza. El gasto cuelga de un DEPARTAMENTO —alguien
-- responde por él— y el ingreso de una LÍNEA: acá las líneas son canales de cobro (Asociaciones,
-- Datáfonos, Depósito de Clientes, Cargo BN…), no departamentos. Meterlo en
-- `presupuesto_departamento` obligaría a inventar un departamento dueño de cada canal, o a hacer
-- `departamento_id` nulo y dejar la tabla significando dos cosas según una columna vacía.
--
-- ── EL PATRÓN DEL TOTAL Y EL DESGLOSE, QUE YA SE CONOCE ─────────────────────────────────────
--
-- Igual que el subpresupuesto por partida (mig 0066): `clasificacion_id` NULO es el TOTAL del mes;
-- con valor, la meta de esa línea. Las dos clases viven juntas y NO se suman entre sí —sumarlas
-- contaría el mismo colón dos veces—, así que quien totaliza elige una.
--
-- Hay 41 clasificaciones de ingreso: exigir las 41 por mes serían casi 500 celdas al año y la
-- pantalla quedaría vacía para siempre. El total es obligatorio en la práctica; el desglose, de
-- quien lo quiera.

CREATE TABLE presupuesto_ingreso (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id),

    -- YYYY-MM. Mismo formato que el presupuesto de gasto, para que las dos mitades se crucen por
    -- la misma llave sin traducciones.
    periodo text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),

    -- NULO = el total del mes. Con valor = la meta de esa línea de ingreso.
    clasificacion_id uuid REFERENCES clasificacion(id),

    monto_crc numeric(16,2) NOT NULL CHECK (monto_crc >= 0),
    nota      text NOT NULL DEFAULT '',

    creado_por     uuid REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now()
);

-- Un total por mes y una meta por línea y mes. Índices PARCIALES porque un UNIQUE normal sobre
-- una columna nullable deja entrar varios NULL: sin el primero, el mismo mes podría tener dos
-- totales distintos y nadie sabría cuál es la meta.
CREATE UNIQUE INDEX ux_presup_ingreso_total
    ON presupuesto_ingreso (empresa_id, periodo)
    WHERE clasificacion_id IS NULL;

CREATE UNIQUE INDEX ux_presup_ingreso_linea
    ON presupuesto_ingreso (empresa_id, periodo, clasificacion_id)
    WHERE clasificacion_id IS NOT NULL;

CREATE INDEX ix_presup_ingreso_periodo ON presupuesto_ingreso (empresa_id, periodo);

COMMENT ON TABLE presupuesto_ingreso IS
    'Meta de ingreso por mes. clasificacion_id NULO = total del mes; con valor = meta de esa línea de ingreso. Las dos clases NO se suman entre sí.';

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- Se cuelga de los que YA gobiernan el presupuesto de gasto: ver con `bancos.ver_control` y
-- editar con `bancos.ajustes`. Es la otra mitad de la misma decisión —cuánto esperamos que
-- entre y cuánto autorizamos gastar— y partirla en dos permisos obligaría a mantener dos
-- listas para un solo acto de gestión.
--
-- No hay permiso nuevo a propósito: el catálogo ya tiene 106 y agregar uno por cada mitad de una
-- función lo vuelve imposible de administrar.
