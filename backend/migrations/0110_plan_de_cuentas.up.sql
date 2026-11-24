-- 0109 · El plan de cuentas entra al sistema. FASE 1 de la contabilidad.
--
-- ── QUÉ ES Y QUÉ NO ES ──────────────────────────────────────────────────────────────────────
--
-- Esto NO contabiliza nada. Es una tabla que, al terminar esta migración, ningún módulo lee: el
-- importador de bancos, CxP, CxC y nómina siguen exactamente igual. El valor viene después, en el
-- mapeo (Fase 2) y en el motor de asientos (Fase 3).
--
-- Se construye primero porque todo lo demás le cuelga: sin cuentas no hay asiento, sin asiento no
-- hay mayor, sin mayor no hay estados financieros.
--
-- ── POR QUÉ `empresa_id` ADMITE NULO ────────────────────────────────────────────────────────
--
-- Porque la decisión «¿plan único del grupo o uno por empresa?» TODAVÍA NO ESTÁ TOMADA, y este
-- modelo no la fuerza:
--
--   · `empresa_id IS NULL`  → cuenta del GRUPO: la ven y la usan las tres empresas.
--   · `empresa_id = X`      → cuenta propia de esa empresa.
--
-- Las dos conviven, así que también soporta el caso intermedio —un plan base común más cuentas
-- propias donde cada empresa difiere— que es el que la experiencia dice que termina pasando.
-- Es el mismo patrón que ya usan los roles base (globales) frente a los roles por empresa.
--
-- Decidir mal esto y descubrirlo en la Fase 3 obligaría a reescribir el modelo con asientos ya
-- registrados encima. Así, la decisión se toma al CARGAR, no al construir, y se puede cambiar
-- moviendo filas.
--
-- ── LAS DOS REGLAS QUE LA BASE HACE CUMPLIR ─────────────────────────────────────────────────
--
-- 1 · Una cuenta CON HIJAS no acepta movimiento. Si aceptara, el mismo dinero quedaría sumado dos
--     veces al totalizar el árbol —una en la hija y otra en la madre— y ningún balance cuadraría.
--     Es LA regla de un plan de cuentas y se verifica con un disparador, no con buena voluntad.
--
-- 2 · El código es único por empresa. Es la identidad de la cuenta: lo que el contador dicta, lo
--     que viene en los asientos y lo que amarra con Softland.
--
-- ── LA COLUMNA DE SOFTLAND NO ES DECORATIVA ─────────────────────────────────────────────────
--
-- 228 de las 258 cuentas del catálogo del Director Financiero traen su equivalencia en Softland,
-- y 30 cambian de código respecto de él. Esa columna es el único puente hacia los SALDOS
-- INICIALES, que es lo que la Fase 4 va a necesitar y no se puede inventar.

CREATE TABLE IF NOT EXISTS cuenta_contable (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- NULO = cuenta del grupo, visible para todas las empresas. Ver arriba.
    empresa_id  uuid REFERENCES empresa(id),
    -- El código tal como lo escribe el contador: «1-01-01-001-000». Texto y no número: los ceros
    -- a la izquierda y los guiones son parte del código, y un numérico los perdería.
    codigo      text        NOT NULL,
    nombre      text        NOT NULL,
    -- La cuenta madre. Nula en las raíces (ACTIVOS, PASIVOS, …). Se calcula al importar a partir
    -- del código y se GUARDA: derivarla en cada consulta ataría el árbol a la máscara del código,
    -- y la máscara puede cambiar sin que cambie la jerarquía.
    padre_id    uuid REFERENCES cuenta_contable(id),
    -- Clase contable, el primer segmento del código. Decide el signo en los estados financieros.
    clase       text        NOT NULL CHECK (clase IN ('ACTIVO','PASIVO','PATRIMONIO','INGRESO','COSTO','GASTO','ORDEN')),
    -- Deudora: aumenta por el DEBE (activo, costo, gasto). Acreedora: por el HABER.
    -- Se deriva de la clase al importar, pero se guarda porque hay excepciones legítimas
    -- (las cuentas correctoras, como la depreciación acumulada, que es activo y acreedora).
    naturaleza  text        NOT NULL CHECK (naturaleza IN ('DEUDORA','ACREEDORA')),
    -- Solo una cuenta de MOVIMIENTO admite líneas de asiento. Las de agrupación totalizan a sus
    -- hijas. Lo hace cumplir el disparador de abajo.
    acepta_movimiento boolean NOT NULL DEFAULT false,
    -- Nivel en el árbol, 1 para las raíces. Se guarda para poder ordenar e indentar sin recorrer.
    nivel       int         NOT NULL CHECK (nivel BETWEEN 1 AND 9),
    -- El código de la MISMA cuenta en Softland, cuando existe. Puente a los saldos iniciales.
    codigo_softland text    NOT NULL DEFAULT '',
    -- Apagar una cuenta no la borra: lo ya contabilizado en ella tiene que seguir explicándose.
    activo      boolean     NOT NULL DEFAULT true,
    creado_por  uuid REFERENCES usuario(id),
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE cuenta_contable IS
  'Plan de cuentas. Fase 1 de la contabilidad: ningún módulo lo lee todavía. empresa_id NULO = '
  'cuenta del grupo; con empresa = propia de esa empresa.';
COMMENT ON COLUMN cuenta_contable.acepta_movimiento IS
  'Solo estas admiten líneas de asiento. Una cuenta con hijas no puede aceptarlo: sumaría el '
  'mismo dinero dos veces al totalizar el árbol. Lo impide el disparador trg_cuenta_sin_hijas.';
COMMENT ON COLUMN cuenta_contable.codigo_softland IS
  'El código de esta misma cuenta en Softland. Es el puente para traer los saldos iniciales.';

-- El código identifica la cuenta. Único por empresa, y aparte único entre las del grupo: dos
-- índices parciales porque un UNIQUE normal trata cada NULL como distinto y dejaría pasar dos
-- cuentas del grupo con el mismo código.
CREATE UNIQUE INDEX IF NOT EXISTS ux_cuenta_codigo_empresa
    ON cuenta_contable (empresa_id, codigo) WHERE empresa_id IS NOT NULL;
CREATE UNIQUE INDEX IF NOT EXISTS ux_cuenta_codigo_grupo
    ON cuenta_contable (codigo) WHERE empresa_id IS NULL;

CREATE INDEX IF NOT EXISTS ix_cuenta_padre ON cuenta_contable (padre_id);
CREATE INDEX IF NOT EXISTS ix_cuenta_empresa_codigo ON cuenta_contable (empresa_id, codigo);

-- ── La regla del árbol, en la base ──────────────────────────────────────────────────────────
--
-- Se hace cumplir con un disparador y no con un CHECK porque mira OTRAS filas: un CHECK solo ve
-- la suya. Cubre los dos lados del mismo error:
--
--   · marcar como de movimiento una cuenta que ya tiene hijas;
--   · colgarle una hija a una cuenta que acepta movimiento.
--
-- Sin el segundo, el árbol se rompe por el otro extremo y nadie lo nota hasta que el balance no
-- cuadra, que es meses después.
CREATE OR REPLACE FUNCTION fn_cuenta_sin_hijas() RETURNS trigger AS $$
BEGIN
    IF NEW.acepta_movimiento AND EXISTS (SELECT 1 FROM cuenta_contable WHERE padre_id = NEW.id) THEN
        RAISE EXCEPTION 'la cuenta % tiene cuentas hijas, así que no puede aceptar movimiento: sumaría el mismo dinero dos veces', NEW.codigo
            USING ERRCODE = 'check_violation';
    END IF;
    IF NEW.padre_id IS NOT NULL
       AND (SELECT acepta_movimiento FROM cuenta_contable WHERE id = NEW.padre_id) THEN
        RAISE EXCEPTION 'la cuenta madre acepta movimiento, así que no puede tener hijas: sumaría el mismo dinero dos veces'
            USING ERRCODE = 'check_violation';
    END IF;
    RETURN NEW;
END;
$$ LANGUAGE plpgsql;

DROP TRIGGER IF EXISTS trg_cuenta_sin_hijas ON cuenta_contable;
CREATE TRIGGER trg_cuenta_sin_hijas
    BEFORE INSERT OR UPDATE ON cuenta_contable
    FOR EACH ROW EXECUTE FUNCTION fn_cuenta_sin_hijas();

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- Propio y en su propio módulo: el plan de cuentas no es un catálogo de análisis como el de
-- Bancos. Define dónde se registra cada colón, y una cuenta mal clasificada mueve el resultado
-- del período. No se reparte con la operación.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('conta.plan_cuentas',
     'Contabilidad',
     'Plan de cuentas',
     'Ver y mantener el catálogo contable: crear cuentas, moverlas en el árbol e importarlo desde Excel.')
ON CONFLICT (codigo) DO NOTHING;

-- Se siembra sobre las parejas (empresa, rol) que ya pueden cerrar el período de Bancos: es la
-- misma clase de responsabilidad —el cierre contable— y no se le estrena la facultad a nadie.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso cierre ON cierre.id = rp.permiso_id AND cierre.codigo = 'bancos.cerrar_periodo'
CROSS JOIN permiso p
WHERE p.codigo = 'conta.plan_cuentas'
ON CONFLICT DO NOTHING;
