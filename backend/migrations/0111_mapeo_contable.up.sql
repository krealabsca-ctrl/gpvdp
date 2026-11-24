-- 0110 · El mapeo: qué cuenta contable corresponde a cada partida. FASE 2.
--
-- ── EL TRABAJO QUE ESTO HABILITA ────────────────────────────────────────────────────────────
--
-- El plan de cuentas (mig 0109) ya existe en el sistema y no sirve de nada solo. Lo que falta es
-- la equivalencia con las partidas que la operación ya usa todos los días, y esa equivalencia no
-- existe en ninguna parte: ni en Softland, ni en el ERP, ni en el Excel del contador.
--
-- Son 332 clasificaciones con movimientos en las tres empresas (159 Memorial Pets, 131 Valle de
-- Paz, 42 Coopeprofa), medidas el 23-oct-2026.
--
-- ── POR QUÉ UNA TABLA Y NO LA COLUMNA QUE YA HABÍA ──────────────────────────────────────────
--
-- `clasificacion.cuenta_contable_futura` existe desde el diseño original y está vacía en las 398
-- filas. Era el gancho correcto para una idea más simple, pero se queda corta en tres cosas que
-- la Fase 2 necesita:
--
--   1. ES TEXTO, SIN LLAVE. Un código escrito a mano queda colgando el día que la cuenta se
--      renombra o se borra, y nadie se entera hasta que un asiento apunte a la nada.
--   2. NO PUEDE DECIR «ESTA NO SE CONTABILIZA». Hay clasificaciones que no son un asiento —un
--      traslado entre cuentas propias, una partida de control— y declararlo con su motivo es
--      parte del trabajo, no una omisión. Una columna vacía no distingue «falta decidir» de
--      «se decidió que no va».
--   3. NO GUARDA QUIÉN NI CUÁNDO. Mapear es criterio contable: dentro de seis meses, cuando un
--      auditor pregunte por qué la caja chica va a una cuenta de activo, la respuesta tiene que
--      estar escrita.
--
-- La columna NO se borra: el backend la acepta al crear una clasificación y quitarla tocaría
-- código que hoy funciona. Queda marcada como superada por esta tabla.
--
-- ── LAS DOS INVARIANTES ─────────────────────────────────────────────────────────────────────
--
-- 1 · O TIENE CUENTA, O TIENE MOTIVO. Nunca las dos, nunca ninguna. Es lo que convierte el mapeo
--     en una lista que se puede terminar: cada clasificación o está resuelta con su cuenta, o
--     está resuelta con su explicación. Lo hace cumplir un CHECK.
--
-- 2 · NO SE MAPEA A UNA CUENTA DE AGRUPACIÓN. Solo las cuentas de movimiento admiten asientos
--     (mig 0109). Mapear a una cuenta madre produciría asientos que rompen el árbol, y el error
--     aparecería recién al cuadrar el balance. Lo hace cumplir un disparador, porque mira otra
--     tabla.

CREATE TABLE IF NOT EXISTS mapeo_contable (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       uuid NOT NULL REFERENCES empresa(id),
    clasificacion_id uuid NOT NULL REFERENCES clasificacion(id) ON DELETE CASCADE,
    -- La cuenta a la que va. NULA cuando la clasificación se declaró no contabilizable.
    -- RESTRICT y no CASCADE: borrar una cuenta que alguien mapeó tiene que doler, no pasar
    -- inadvertido dejando el mapeo en nada.
    cuenta_id        uuid REFERENCES cuenta_contable(id) ON DELETE RESTRICT,
    estado           text NOT NULL CHECK (estado IN ('MAPEADA','NO_CONTABILIZABLE')),
    -- Obligatorio cuando no se contabiliza. Es la explicación que un auditor va a pedir.
    motivo           text NOT NULL DEFAULT '',
    decidido_por     uuid REFERENCES usuario(id),
    decidido_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en   timestamptz NOT NULL DEFAULT now(),

    -- INVARIANTE 1: o cuenta, o motivo. Nunca las dos, nunca ninguna.
    CONSTRAINT mapeo_cuenta_o_motivo CHECK (
        (estado = 'MAPEADA'            AND cuenta_id IS NOT NULL)
     OR (estado = 'NO_CONTABILIZABLE'  AND cuenta_id IS NULL AND btrim(motivo) <> '')
    )
);

COMMENT ON TABLE mapeo_contable IS
  'Qué cuenta contable corresponde a cada clasificación del ERP. Fase 2: todavía no genera '
  'asientos. Una clasificación o tiene cuenta, o tiene el motivo por el que no se contabiliza.';
COMMENT ON COLUMN mapeo_contable.motivo IS
  'Por qué esta partida no se contabiliza. Obligatorio en NO_CONTABILIZABLE: es la respuesta a '
  'la pregunta que un auditor hace dentro de seis meses.';

COMMENT ON COLUMN clasificacion.cuenta_contable_futura IS
  'SUPERADA por la tabla mapeo_contable (mig 0110). Era texto sin llave foránea, no podía '
  'declarar una partida como no contabilizable ni registrar quién lo decidió. Se conserva porque '
  'el alta de clasificaciones todavía la acepta; el mapeo de verdad vive en mapeo_contable.';

-- Una clasificación tiene UN mapeo. Dos equivalencias para la misma partida es no tener ninguna.
CREATE UNIQUE INDEX IF NOT EXISTS ux_mapeo_clasificacion
    ON mapeo_contable (empresa_id, clasificacion_id);
CREATE INDEX IF NOT EXISTS ix_mapeo_cuenta ON mapeo_contable (cuenta_id);

-- ── INVARIANTE 2: solo cuentas de movimiento ────────────────────────────────────────────────
--
-- Con disparador y no con CHECK porque mira `cuenta_contable`, que es otra tabla. Cubre también
-- el caso de la cuenta que era hoja y gana hijas después: entonces deja de aceptar movimiento, y
-- los mapeos que la apuntaban quedarían mal. Ese segundo caso lo vigila la Fase 3 al generar
-- asientos; acá se impide crear el mapeo malo de entrada.
CREATE OR REPLACE FUNCTION fn_mapeo_cuenta_de_movimiento() RETURNS trigger AS $$
DECLARE
    v_acepta boolean;
    v_empresa uuid;
    v_codigo text;
BEGIN
    IF NEW.cuenta_id IS NULL THEN
        RETURN NEW;
    END IF;
    SELECT acepta_movimiento, empresa_id, codigo
      INTO v_acepta, v_empresa, v_codigo
      FROM cuenta_contable WHERE id = NEW.cuenta_id;

    IF NOT v_acepta THEN
        RAISE EXCEPTION 'la cuenta % agrupa a otras, así que no admite asientos: elegí una cuenta de movimiento', v_codigo
            USING ERRCODE = 'check_violation';
    END IF;
    -- Y tiene que ser una cuenta que esta empresa pueda ver: la suya o la del grupo. Mapear
    -- contra la cuenta de otra empresa cruzaría la contabilidad de dos compañías.
    IF v_empresa IS NOT NULL AND v_empresa <> NEW.empresa_id THEN
        RAISE EXCEPTION 'la cuenta % es de otra empresa', v_codigo
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_mapeo_cuenta_de_movimiento ON mapeo_contable;
CREATE TRIGGER trg_mapeo_cuenta_de_movimiento
    BEFORE INSERT OR UPDATE ON mapeo_contable
    FOR EACH ROW EXECUTE FUNCTION fn_mapeo_cuenta_de_movimiento();

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- Propio y distinto de `conta.plan_cuentas`: son dos responsabilidades. Mantener el plan es
-- estructura y se toca pocas veces; mapear es trabajo del contador, repetitivo, y va a crecer
-- cada vez que la operación invente una partida nueva. Separarlos permite que alguien mapee sin
-- poder reorganizar el plan de cuentas.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('conta.mapeo',
     'Contabilidad',
     'Mapeo contable',
     'Decidir a qué cuenta contable corresponde cada partida del ERP, o declararla no contabilizable con su motivo.')
ON CONFLICT (codigo) DO NOTHING;

-- Lo recibe quien ya puede mantener el plan de cuentas: es el mismo criterio profesional.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso plan ON plan.id = rp.permiso_id AND plan.codigo = 'conta.plan_cuentas'
CROSS JOIN permiso p
WHERE p.codigo = 'conta.mapeo'
ON CONFLICT DO NOTHING;
