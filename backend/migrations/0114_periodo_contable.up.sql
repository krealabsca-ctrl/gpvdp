-- 0113 · El período contable: apertura, cierre y reapertura. FASE 4.
--
-- ── POR QUÉ UNA TABLA PROPIA Y NO `periodo_cierre` ──────────────────────────────────────────
--
-- `periodo_cierre` ya existe y es de BANCOS: dice «este mes ya está clasificado, no se toca más»
-- y guarda cuántos movimientos quedaron sin identificar al cerrarlo. Protege la clasificación.
--
-- Esto protege otra cosa: el LIBRO. «Los estados financieros de este mes ya se emitieron, no
-- entran más asientos». Son dos decisiones distintas, con reglas distintas y momentos distintos
-- —un mes puede estar clasificado y todavía no contabilizado—, y juntarlas en una sola bandera
-- haría que cerrar la clasificación congelara la contabilidad sin que nadie lo pidiera.
--
-- ── LA INVARIANTE ───────────────────────────────────────────────────────────────────────────
--
-- UN PERÍODO CERRADO NO RECIBE ASIENTOS. Ni nuevos, ni modificados, ni anulados: si el saldo de
-- un mes ya se publicó, cualquier cambio posterior hace que dos personas con el mismo reporte
-- vean números distintos. La hace cumplir un disparador, no el servicio, para que valga también
-- desde un script o un `psql` a mano.
--
-- Para corregir un mes cerrado hay que REABRIRLO, y eso exige un motivo que queda escrito.

CREATE TABLE IF NOT EXISTS periodo_contable (
    id         uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id uuid NOT NULL REFERENCES empresa(id),
    anio       int  NOT NULL CHECK (anio BETWEEN 2000 AND 2100),
    mes        int  NOT NULL CHECK (mes BETWEEN 1 AND 12),

    estado     text NOT NULL DEFAULT 'ABIERTO' CHECK (estado IN ('ABIERTO', 'CERRADO')),

    cerrado_en  timestamptz,
    cerrado_por uuid REFERENCES usuario(id),
    -- Cuánto tenía el libro al cerrar. Se guarda para poder detectar después si algo cambió
    -- —no debería, pero el día que no cuadre conviene saber desde cuándo—.
    debe_al_cierre  numeric(18,2),
    haber_al_cierre numeric(18,2),
    asientos_al_cierre int,

    -- La reapertura queda registrada CON SU MOTIVO: es la pregunta que un auditor hace primero
    -- cuando ve un período que se cerró y se volvió a abrir.
    reabierto_en     timestamptz,
    reabierto_por    uuid REFERENCES usuario(id),
    reabierto_motivo text NOT NULL DEFAULT '',

    creado_en timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT periodo_reabierto_con_motivo CHECK (
        reabierto_en IS NULL OR btrim(reabierto_motivo) <> ''
    )
);

COMMENT ON TABLE periodo_contable IS
  'Cierre del LIBRO por mes. Distinto de periodo_cierre, que es el cierre de la clasificación '
  'bancaria. Un período cerrado no admite asientos nuevos, modificados ni anulados.';

CREATE UNIQUE INDEX IF NOT EXISTS ux_periodo_contable
    ON periodo_contable (empresa_id, anio, mes);

-- ── El disparador que hace cumplir el cierre ────────────────────────────────────────────────
--
-- Mira la FECHA del asiento, no cuándo se escribe: un asiento con fecha de agosto entra al
-- período de agosto aunque se cargue en octubre, y es justamente ese el que hay que frenar.
CREATE OR REPLACE FUNCTION fn_periodo_no_cerrado() RETURNS trigger AS $$
DECLARE
    v_empresa uuid;
    v_fecha   date;
    v_estado  text;
BEGIN
    -- Según la tabla: el asiento trae empresa y fecha; la línea las hereda de su cabecera.
    IF TG_TABLE_NAME = 'asiento' THEN
        v_empresa := COALESCE(NEW.empresa_id, OLD.empresa_id);
        v_fecha   := COALESCE(NEW.fecha, OLD.fecha);
    ELSE
        SELECT a.empresa_id, a.fecha INTO v_empresa, v_fecha
        FROM asiento a WHERE a.id = COALESCE(NEW.asiento_id, OLD.asiento_id);
        -- Si la cabecera ya no está, la línea se va con ella por CASCADE: nada que proteger.
        IF NOT FOUND THEN
            RETURN COALESCE(NEW, OLD);
        END IF;
    END IF;

    SELECT estado INTO v_estado
    FROM periodo_contable
    WHERE empresa_id = v_empresa
      AND anio = EXTRACT(YEAR FROM v_fecha)::int
      AND mes  = EXTRACT(MONTH FROM v_fecha)::int;

    IF v_estado = 'CERRADO' THEN
        RAISE EXCEPTION 'el período % ya está cerrado: para corregirlo hay que reabrirlo, y eso queda registrado',
            to_char(v_fecha, 'MM/YYYY')
            USING ERRCODE = 'check_violation';
    END IF;

    RETURN COALESCE(NEW, OLD);
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_asiento_periodo_abierto ON asiento;
CREATE TRIGGER trg_asiento_periodo_abierto
    BEFORE INSERT OR UPDATE OR DELETE ON asiento
    FOR EACH ROW EXECUTE FUNCTION fn_periodo_no_cerrado();

-- También sobre la línea: agregar o cambiar una línea de un asiento viejo mueve el saldo del mes
-- igual que crear el asiento entero.
DROP TRIGGER IF EXISTS trg_linea_periodo_abierto ON asiento_linea;
CREATE TRIGGER trg_linea_periodo_abierto
    BEFORE INSERT OR UPDATE OR DELETE ON asiento_linea
    FOR EACH ROW EXECUTE FUNCTION fn_periodo_no_cerrado();

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- Cerrar un período congela los estados financieros de un mes para toda la empresa, y reabrirlo
-- los descongela. No es una tarea de quien registra: es de quien responde por el número.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('conta.periodos',
     'Contabilidad',
     'Cerrar y reabrir períodos',
     'Cerrar el libro de un mes para que no entren más asientos, y reabrirlo con un motivo registrado. También carga los saldos iniciales.')
ON CONFLICT (codigo) DO NOTHING;

-- Lo recibe quien ya puede generar asientos: es el paso siguiente del mismo trabajo.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso base ON base.id = rp.permiso_id AND base.codigo = 'conta.generar'
CROSS JOIN permiso p
WHERE p.codigo = 'conta.periodos'
ON CONFLICT DO NOTHING;
