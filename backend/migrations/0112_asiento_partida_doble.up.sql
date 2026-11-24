-- 0111 · El asiento contable y la pata que le faltaba al mapeo. FASE 3A.
--
-- ── QUÉ ES 3A Y QUÉ NO ──────────────────────────────────────────────────────────────────────
--
-- La Fase 3 completa es el motor de asientos. Se parte en dos porque la mitad depende de una
-- decisión que todavía no está tomada (A3, la matriz de reglas de contabilización: qué se debita
-- y qué se acredita en cada tipo de transacción) y la otra mitad NO depende de ella:
--
--   3A (esta migración)  Es cierto se contabilice por caja o por devengo:
--                        · la estructura del asiento, con partida doble imposible de violar
--                        · el enlace cuenta bancaria → cuenta contable
--                        · la idempotencia por documento de origen
--   3B (pendiente de A3) El generador: de qué documento sale cada asiento y en qué dirección.
--
-- ESTA MIGRACIÓN NO GENERA NINGÚN ASIENTO. Crea el lugar donde van a vivir y lo que hace falta
-- para que puedan existir; llenarlos es 3B.
--
-- ── EL HUECO QUE CIERRA ─────────────────────────────────────────────────────────────────────
--
-- Un asiento necesita DOS cuentas. La Fase 2 (mig 0110) mapeó una —la clasificación de la
-- partida—, y la otra es la cuenta contable del banco. Ese enlace no existía: `cuenta_bancaria`
-- no tiene ninguna columna hacia `cuenta_contable`. Sin él, de un movimiento bancario sale media
-- partida doble, que no es un asiento.
--
-- Medido en Valle de Paz el 26-oct-2026: 16 cuentas bancarias; el plan contable ya trae las
-- cuentas de banco (1-01-02-xxx) con el IBAN escrito en el nombre, y por eso 5 de las 16 se
-- pueden proponer solas comparando IBAN — esas 5 cubren 15.873 de los 20.082 movimientos (79 %).
-- Las otras 11 son decisión manual: 8 no tienen IBAN cargado en el ERP (las de BN, Promerica y
-- Scotiabank) y 3 lo tienen pero sin cuenta contable que calce.

-- ── El enlace que faltaba ───────────────────────────────────────────────────────────────────
--
-- Tabla propia y no una columna en `cuenta_bancaria` por dos razones: esa tabla es del módulo
-- Bancos y ninguna fase de contabilidad modifica tablas de otros módulos; y mapear es criterio
-- contable, así que hay que saber quién lo decidió y cuándo.
CREATE TABLE IF NOT EXISTS mapeo_cuenta_bancaria (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id         uuid NOT NULL REFERENCES empresa(id),
    cuenta_bancaria_id uuid NOT NULL REFERENCES cuenta_bancaria(id) ON DELETE CASCADE,
    -- RESTRICT y no CASCADE, igual que en la 0110: borrar una cuenta contable que alguien mapeó
    -- tiene que doler, no pasar inadvertido dejando el mapeo en nada.
    cuenta_id          uuid NOT NULL REFERENCES cuenta_contable(id) ON DELETE RESTRICT,
    decidido_por       uuid REFERENCES usuario(id),
    decidido_en        timestamptz NOT NULL DEFAULT now(),
    actualizado_en     timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE mapeo_cuenta_bancaria IS
  'Qué cuenta contable representa cada cuenta bancaria del ERP. Es la contrapartida de todo '
  'asiento que toca el banco. Sin esto, de un movimiento bancario sale media partida doble.';

-- Una cuenta bancaria tiene UNA cuenta contable. Dos sería no tener ninguna.
CREATE UNIQUE INDEX IF NOT EXISTS ux_mapeo_cb_cuenta
    ON mapeo_cuenta_bancaria (empresa_id, cuenta_bancaria_id);
CREATE INDEX IF NOT EXISTS ix_mapeo_cb_contable ON mapeo_cuenta_bancaria (cuenta_id);

-- A PROPÓSITO NO HAY UNIQUE SOBRE cuenta_id. Dos cuentas bancarias apuntando a la misma cuenta
-- contable mezcla dos bancos en el mismo mayor, y casi siempre es un error —en el catálogo real
-- cada cuenta tiene la suya, con el IBAN en el nombre—. Pero «casi siempre» no es «siempre»: si
-- el contador quiere agrupar dos cuentas del mismo banco, es su criterio. Mismo trato que el
-- cruce de naturaleza de la 0110: la pantalla lo AVISA y no lo impide.

-- Se reutiliza el disparador de la 0110 tal cual: lee NEW.cuenta_id y NEW.empresa_id, que esta
-- tabla también tiene. La regla es la misma —solo cuentas de movimiento, y de esta empresa o del
-- grupo— y duplicarla sería arriesgar que las dos copias se separen.
DROP TRIGGER IF EXISTS trg_mapeo_cb_cuenta_de_movimiento ON mapeo_cuenta_bancaria;
CREATE TRIGGER trg_mapeo_cb_cuenta_de_movimiento
    BEFORE INSERT OR UPDATE ON mapeo_cuenta_bancaria
    FOR EACH ROW EXECUTE FUNCTION fn_mapeo_cuenta_de_movimiento();

-- ── El consecutivo del asiento ──────────────────────────────────────────────────────────────
--
-- Un libro diario se cita por número de asiento, así que no puede haber huecos ni repetidos. El
-- `UPDATE … RETURNING` toma el lock de la fila y serializa a los que pidan número a la vez; un
-- MAX(numero)+1 tendría carrera.
CREATE TABLE IF NOT EXISTS conta_consecutivo (
    empresa_id uuid NOT NULL REFERENCES empresa(id),
    ambito     text NOT NULL,
    siguiente  bigint NOT NULL DEFAULT 1,
    PRIMARY KEY (empresa_id, ambito)
);

-- ── El asiento ──────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS asiento (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id),
    numero      bigint NOT NULL,
    fecha       date NOT NULL,
    -- La glosa es lo que alguien lee en el diario seis meses después. Vacía, el asiento existe
    -- pero no se puede explicar.
    glosa       text NOT NULL CHECK (btrim(glosa) <> ''),

    -- De qué documento salió. Es lo que hace que regenerar no duplique.
    origen_tipo text NOT NULL CHECK (origen_tipo IN (
        'MANUAL',              -- lo escribió una persona
        'MOVIMIENTO_BANCARIO',
        'CXP_FACTURA',
        'CXP_PAGO',
        'CAJA_CHICA_VALE',
        'NOMINA',
        'CXC_CARGO',
        'CXC_COBRO',
        'APERTURA'             -- saldos iniciales (Fase 4)
    )),
    -- Los orígenes de arriba son los documentos que HOY existen en el sistema. Cuál de ellos
    -- genera asiento, y en qué dirección, lo decide A3: esta lista dice de dónde PUEDE venir un
    -- asiento, no de dónde viene. Sumar un origen nuevo es agregar un valor acá.
    origen_id   uuid,

    estado      text NOT NULL DEFAULT 'BORRADOR'
                CHECK (estado IN ('BORRADOR', 'ASENTADO', 'ANULADO')),
    -- Un asiento asentado NO se borra ni se corrige: se anula y se vuelve a hacer. Es la regla
    -- del libro, no una preferencia: borrar deja un hueco en la numeración y un mayor que ya no
    -- se puede reconstruir. `anulado_motivo` es lo que un auditor va a pedir.
    anulado_en     timestamptz,
    anulado_por    uuid REFERENCES usuario(id),
    anulado_motivo text NOT NULL DEFAULT '',

    creado_por  uuid REFERENCES usuario(id),
    creado_en   timestamptz NOT NULL DEFAULT now(),
    asentado_en timestamptz,

    -- Un asiento manual no sale de ningún documento; uno generado siempre apunta al suyo. Sin
    -- esto, un asiento generado sin origen sería invisible para la idempotencia y se duplicaría
    -- en la siguiente corrida.
    CONSTRAINT asiento_origen_coherente CHECK (
        (origen_tipo =  'MANUAL' AND origen_id IS NULL)
     OR (origen_tipo <> 'MANUAL' AND origen_id IS NOT NULL)
    ),
    CONSTRAINT asiento_anulado_con_motivo CHECK (
        estado <> 'ANULADO' OR btrim(anulado_motivo) <> ''
    )
);

COMMENT ON TABLE asiento IS
  'Cabecera del asiento contable. La partida doble la garantiza un disparador DIFERIDO, no el '
  'código: ver fn_verificar_asiento. Esta migración no genera ninguno; eso es la Fase 3B.';

CREATE UNIQUE INDEX IF NOT EXISTS ux_asiento_numero ON asiento (empresa_id, numero);
CREATE INDEX IF NOT EXISTS ix_asiento_fecha ON asiento (empresa_id, fecha);

-- IDEMPOTENCIA: un documento tiene UN asiento vivo. Índice parcial por dos razones a la vez —
-- deja fuera los manuales (origen_id nulo, y un UNIQUE normal trata cada NULL como distinto pero
-- igual estorbaría al leer) y deja fuera los anulados, porque después de anular hay que poder
-- regenerar. Sin el segundo filtro, un asiento mal generado bloquearía su documento para siempre.
CREATE UNIQUE INDEX IF NOT EXISTS ux_asiento_origen
    ON asiento (empresa_id, origen_tipo, origen_id)
    WHERE origen_id IS NOT NULL AND estado <> 'ANULADO';

-- ── La línea ────────────────────────────────────────────────────────────────────────────────
CREATE TABLE IF NOT EXISTS asiento_linea (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    asiento_id uuid NOT NULL REFERENCES asiento(id) ON DELETE CASCADE,
    cuenta_id  uuid NOT NULL REFERENCES cuenta_contable(id) ON DELETE RESTRICT,

    -- EN COLONES, SIEMPRE. El mayor se suma columna abajo, y una columna que mezcla colones con
    -- dólares no suma nada. El monto en su moneda queda al lado para poder rastrearlo; el
    -- movimiento bancario ya trae monto_crc, monto_original, moneda_original y tc_aplicado.
    debe       numeric(18,2) NOT NULL DEFAULT 0 CHECK (debe  >= 0),
    haber      numeric(18,2) NOT NULL DEFAULT 0 CHECK (haber >= 0),

    moneda_origen text NOT NULL DEFAULT 'CRC' CHECK (moneda_origen IN ('CRC','USD')),
    monto_origen  numeric(18,2),
    tc_aplicado   numeric(18,6),

    detalle    text NOT NULL DEFAULT '',
    orden      int  NOT NULL DEFAULT 0,

    -- Una línea va al debe O al haber. Las dos a la vez se netea sola y esconde el movimiento;
    -- ninguna de las dos es una línea que no dice nada.
    CONSTRAINT linea_debe_o_haber CHECK (
        (debe > 0 AND haber = 0) OR (haber > 0 AND debe = 0)
    )
);

CREATE INDEX IF NOT EXISTS ix_linea_asiento ON asiento_linea (asiento_id);
-- El mayor se lee POR CUENTA: sin este índice, armar el mayor de una cuenta recorre todas las
-- líneas de todos los asientos.
CREATE INDEX IF NOT EXISTS ix_linea_cuenta ON asiento_linea (cuenta_id);

-- ── LA INVARIANTE: imposible dejar un asiento descuadrado ───────────────────────────────────
--
-- Tiene que ser DIFERIDA. Las líneas entran una por una, así que inmediatamente después de la
-- primera el asiento está descuadrado por construcción: un disparador normal rechazaría todo
-- asiento que alguna vez intentara existir. `DEFERRABLE INITIALLY DEFERRED` lo comprueba al
-- COMMIT, que es el único momento en que la pregunta tiene sentido.
--
-- Y va en la BASE y no en Go porque es la única forma de que valga para todo escritor: un script
-- de corrección, una carga masiva, un `psql` a mano. Un `if` en el servicio protege al servicio.
CREATE OR REPLACE FUNCTION fn_verificar_asiento(p_asiento uuid) RETURNS void AS $$
DECLARE
    v_debe   numeric;
    v_haber  numeric;
    v_lineas int;
    v_numero bigint;
BEGIN
    -- Si el asiento ya no existe, sus líneas se fueron con él por CASCADE y no hay nada que
    -- cuadrar. Sin esta salida, borrar un asiento fallaría al commit.
    SELECT numero INTO v_numero FROM asiento WHERE id = p_asiento;
    IF NOT FOUND THEN
        RETURN;
    END IF;

    SELECT COALESCE(sum(debe), 0), COALESCE(sum(haber), 0), count(*)
      INTO v_debe, v_haber, v_lineas
      FROM asiento_linea WHERE asiento_id = p_asiento;

    IF v_lineas < 2 THEN
        RAISE EXCEPTION 'el asiento % tiene % línea(s): hace falta al menos una al debe y otra al haber',
            v_numero, v_lineas USING ERRCODE = 'check_violation';
    END IF;

    IF v_debe <> v_haber THEN
        RAISE EXCEPTION 'el asiento % no cuadra: debe %, haber %, diferencia %',
            v_numero, v_debe, v_haber, v_debe - v_haber USING ERRCODE = 'check_violation';
    END IF;
END;
$$ LANGUAGE plpgsql;

-- Dos disparadores y no uno: la cabecera cubre el asiento que se crea SIN líneas —ahí no hay
-- ninguna fila de línea que dispare nada— y la línea cubre que alguien agregue, cambie o borre
-- líneas después, en otra transacción.
-- La LLAMADA va calificada con `public.` a propósito, y las tablas de adentro NO. Son dos cosas
-- distintas: la función vive en public y se resuelve ahí siempre; las tablas se resuelven por
-- search_path, que es lo que permite que el mismo disparador vigile un esquema de pruebas. Sin
-- el `public.`, un search_path que no incluya public deja el disparador roto y el asiento entra
-- descuadrado sin que nadie se entere.
CREATE OR REPLACE FUNCTION fn_asiento_cuadra_cab() RETURNS trigger AS $$
BEGIN
    PERFORM public.fn_verificar_asiento(COALESCE(NEW.id, OLD.id));
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

CREATE OR REPLACE FUNCTION fn_asiento_cuadra_lin() RETURNS trigger AS $$
BEGIN
    PERFORM public.fn_verificar_asiento(COALESCE(NEW.asiento_id, OLD.asiento_id));
    RETURN NULL;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_asiento_cuadra_cab ON asiento;
CREATE CONSTRAINT TRIGGER trg_asiento_cuadra_cab
    AFTER INSERT OR UPDATE ON asiento
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION fn_asiento_cuadra_cab();

DROP TRIGGER IF EXISTS trg_asiento_cuadra_lin ON asiento_linea;
CREATE CONSTRAINT TRIGGER trg_asiento_cuadra_lin
    AFTER INSERT OR UPDATE OR DELETE ON asiento_linea
    DEFERRABLE INITIALLY DEFERRED
    FOR EACH ROW EXECUTE FUNCTION fn_asiento_cuadra_lin();

-- ── Y la línea solo contra cuentas que admiten asientos ─────────────────────────────────────
--
-- ES LA MISMA REGLA QUE VIGILA EL MAPEO (mig 0110) Y ACÁ IMPORTA MÁS. El mapeo dice a qué cuenta
-- IRÍA una partida; la línea de asiento es la que de verdad mueve el saldo. Un asiento contra una
-- cuenta madre hace que el monto se cuente dos veces al totalizar el árbol —una en la madre, otra
-- en la rama— y el balance deja de cuadrar sin que nada avise.
--
-- No se pudo reutilizar fn_mapeo_cuenta_de_movimiento(): esa lee NEW.empresa_id, y la línea no
-- tiene empresa —la tiene su cabecera—.
--
-- INMEDIATO y no diferido, al revés que el cuadre: la cuenta ya existe cuando la línea se
-- escribe, así que la pregunta se puede contestar en el acto y conviene fallar en la línea
-- exacta en vez de al commit.
CREATE OR REPLACE FUNCTION fn_linea_cuenta_de_movimiento() RETURNS trigger AS $$
DECLARE
    v_acepta          boolean;
    v_empresa_cuenta  uuid;
    v_empresa_asiento uuid;
    v_codigo          text;
BEGIN
    SELECT acepta_movimiento, empresa_id, codigo
      INTO v_acepta, v_empresa_cuenta, v_codigo
      FROM cuenta_contable WHERE id = NEW.cuenta_id;

    IF NOT v_acepta THEN
        RAISE EXCEPTION 'la cuenta % agrupa a otras, así que no admite asientos: elegí una cuenta de movimiento', v_codigo
            USING ERRCODE = 'check_violation';
    END IF;

    -- Y tiene que ser una cuenta que la empresa del asiento pueda ver: la suya o la del grupo.
    -- Asentar contra la cuenta de otra compañía cruzaría dos contabilidades.
    SELECT empresa_id INTO v_empresa_asiento FROM asiento WHERE id = NEW.asiento_id;
    IF v_empresa_cuenta IS NOT NULL AND v_empresa_cuenta <> v_empresa_asiento THEN
        RAISE EXCEPTION 'la cuenta % es de otra empresa', v_codigo
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_linea_cuenta_de_movimiento ON asiento_linea;
CREATE TRIGGER trg_linea_cuenta_de_movimiento
    BEFORE INSERT OR UPDATE ON asiento_linea
    FOR EACH ROW EXECUTE FUNCTION fn_linea_cuenta_de_movimiento();

-- ── Los permisos ────────────────────────────────────────────────────────────────────────────
--
-- Tres y no uno, porque son tres responsabilidades distintas: enlazar los bancos es una tarea de
-- configuración que se hace una vez; LEER el diario y el mayor lo necesita cualquiera que revise
-- la contabilidad, incluido un auditor que no debe poder escribir; y escribir un asiento a mano
-- es la facultad más delicada del módulo —permite mover plata entre cuentas sin que ningún
-- documento lo respalde—.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('conta.bancos',
     'Contabilidad',
     'Cuentas bancarias contables',
     'Decir qué cuenta contable representa cada cuenta bancaria del ERP. Es la contrapartida de todo asiento que toca el banco.'),
    ('conta.libros',
     'Contabilidad',
     'Libro diario y mayor',
     'Consultar los asientos y el movimiento de cada cuenta. Solo lectura.'),
    ('conta.asientos',
     'Contabilidad',
     'Asientos manuales',
     'Escribir y anular asientos a mano. Es la facultad más delicada del módulo: permite registrar movimientos que ningún documento respalda.')
ON CONFLICT (codigo) DO NOTHING;

-- Los recibe quien ya puede mantener el plan de cuentas: es el mismo criterio profesional.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso plan ON plan.id = rp.permiso_id AND plan.codigo = 'conta.plan_cuentas'
CROSS JOIN permiso p
WHERE p.codigo IN ('conta.bancos', 'conta.libros', 'conta.asientos')
ON CONFLICT DO NOTHING;
