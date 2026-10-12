-- Vacaciones: que el colaborador las pida y la jefatura las apruebe, SIN ver nada más.
--
-- El problema, en palabras del Director Financiero (8-oct-2026): «debe haber un permiso que le
-- permita a la jefatura o bien al colaborador solicitar las vacaciones, programarlas, pero que no
-- pueda ver nada más de los empleados o data confidencial, diferenciando el permiso de quien
-- alimenta al que solicita algo».
--
-- Hoy eso es imposible, y está medido: `rrhh.ver` abre QUINCE rutas de una vez. Para que alguien
-- consulte su saldo de vacaciones hay que darle ese permiso, y con él se lleva puesto el salario de
-- todos los empleados, sus deducciones (préstamos, embargos), las corridas completas, los
-- finiquitos y las incapacidades —que son datos de salud—. Es el mismo defecto que tenía «Ver
-- Bancos» antes de la migración 0074, por el cual era imposible darle a la tesorera solo los saldos.
--
-- Esta migración NO parte `rrhh.ver`: abre una puerta nueva al costado. Los dos permisos que crea
-- son independientes y alcanzan solos, así que ni la jefatura ni el colaborador necesitan `rrhh.ver`
-- para nada. Partir `rrhh.ver` sigue siendo deseable y es otro trabajo.

BEGIN;

-- ---------------------------------------------------------------------------
-- 1. El empleado, ligado a su usuario
-- ---------------------------------------------------------------------------
-- Sin esto la pregunta «¿cuáles son MIS vacaciones?» no tiene respuesta posible: la ficha del
-- empleado guardaba el correo como texto suelto y nada decía qué usuario del sistema es esa
-- persona. Es la pieza que convierte un permiso en un alcance.

ALTER TABLE empleado ADD COLUMN usuario_id uuid REFERENCES usuario(id) ON DELETE SET NULL;

COMMENT ON COLUMN empleado.usuario_id IS
  'Usuario del sistema que ES este empleado. Lo usa el autoservicio de vacaciones para resolver el alcance; NULL significa que esa persona no entra al ERP, no que vea de más.';

-- Un usuario no puede ser dos empleados de la misma empresa: si lo fuera, «mis vacaciones»
-- devolvería las de dos personas y el saldo sería la suma de dos acumulaciones distintas.
CREATE UNIQUE INDEX idx_empleado_usuario ON empleado (empresa_id, usuario_id)
  WHERE usuario_id IS NOT NULL;

-- Enlace automático por correo, que es el único dato que las dos tablas comparten hoy.
--
-- Se enlaza SOLO cuando el correo identifica a UNA persona de cada lado. Un correo repetido
-- —dos empleados con el buzón de la sucursal, dos usuarios con el mismo alias— se deja sin
-- enlazar a propósito: enlazar al azar le daría a alguien las vacaciones de otro, y eso no se ve
-- hasta que alguien reclama. Lo que quede suelto se asigna a mano desde la ficha.
WITH correos_unicos AS (
    SELECT lower(trim(e.email)) AS correo
    FROM empleado e
    WHERE COALESCE(trim(e.email), '') <> ''
    GROUP BY 1, e.empresa_id
    HAVING count(*) = 1
),
usuarios_unicos AS (
    SELECT lower(trim(u.email)) AS correo, min(u.id::text)::uuid AS usuario_id
    FROM usuario u
    WHERE COALESCE(trim(u.email), '') <> ''
    GROUP BY 1
    HAVING count(*) = 1
)
UPDATE empleado e
SET usuario_id = uu.usuario_id
FROM correos_unicos cu
JOIN usuarios_unicos uu ON uu.correo = cu.correo
WHERE lower(trim(e.email)) = cu.correo
  AND e.usuario_id IS NULL;

-- ---------------------------------------------------------------------------
-- 2. La jefatura de RRHH
-- ---------------------------------------------------------------------------
-- Decisión del Director (8-oct-2026): jefatura PROPIA, no la de CxP. Ya existe
-- `departamento_validador`, que dice quién valida las facturas de un departamento, y se evaluó
-- reusarla; se descartó porque son dos autoridades distintas: quien aprueba un gasto no es
-- necesariamente quien decide si su gente puede faltar una semana. Amarrarlas haría que cambiar
-- una cambie la otra en silencio.

CREATE TABLE rrhh_jefatura (
    empresa_id      uuid NOT NULL REFERENCES empresa(id),
    departamento_id uuid NOT NULL REFERENCES departamento(id) ON DELETE CASCADE,
    usuario_id      uuid NOT NULL REFERENCES usuario(id) ON DELETE CASCADE,
    -- TITULAR y SUPLENTE con el mismo poder de aprobar: el suplente existe para que una semana de
    -- ausencia del titular no congele las vacaciones de todo un departamento. Quién aprobó queda
    -- escrito en la vacación, así que el rol es informativo y la auditoría no depende de él.
    rol             text NOT NULL DEFAULT 'TITULAR' CHECK (rol IN ('TITULAR', 'SUPLENTE')),
    creado_en       timestamptz NOT NULL DEFAULT now(),
    creado_por      uuid REFERENCES usuario(id),
    PRIMARY KEY (empresa_id, departamento_id, usuario_id)
);

COMMENT ON TABLE rrhh_jefatura IS
  'Quién aprueba las vacaciones de cada departamento. Independiente de departamento_validador (CxP): aprobar un gasto y aprobar días libres son dos autoridades distintas.';

-- El alcance se resuelve preguntando «de qué departamentos es jefe este usuario», así que el
-- índice va por usuario.
CREATE INDEX idx_rrhh_jefatura_usuario ON rrhh_jefatura (empresa_id, usuario_id);

-- ---------------------------------------------------------------------------
-- 3. La vacación pasa a tener estados
-- ---------------------------------------------------------------------------
-- Hasta ahora `vacacion` era un HECHO consumado: RRHH anotaba que alguien disfrutó N días desde
-- tal fecha. Ahora el mismo registro nace antes —cuando se pide— y recorre tres momentos.
--
-- Se extiende la tabla en vez de crear una de solicitudes aparte porque es EL MISMO hecho en
-- distintos momentos, igual que una factura de CxP recorre sus estados. Con dos tablas habría dos
-- fuentes de verdad del saldo y, tarde o temprano, dos respuestas distintas a cuántos días le
-- quedan a alguien.

ALTER TABLE vacacion
    ADD COLUMN estado text NOT NULL DEFAULT 'CONFIRMADA'
        CHECK (estado IN ('SOLICITADA', 'APROBADA', 'CONFIRMADA', 'RECHAZADA')),
    ADD COLUMN solicitada_por  uuid REFERENCES usuario(id),
    ADD COLUMN aprobada_por    uuid REFERENCES usuario(id),
    ADD COLUMN aprobada_en     timestamptz,
    ADD COLUMN confirmada_por  uuid REFERENCES usuario(id),
    ADD COLUMN confirmada_en   timestamptz,
    -- Por qué se rechazó. Va a la cara del colaborador, así que no puede ser un código.
    ADD COLUMN motivo_rechazo  text;

-- EL DEFAULT ES 'CONFIRMADA' Y NO 'SOLICITADA', y de eso depende que esta migración no cambie
-- ningún número: las filas que ya existen las escribió RRHH como disfrute consumado. Si nacieran
-- «solicitadas», los días disfrutados de toda la historia pasarían a estar pendientes de aprobar y
-- cada saldo de la empresa se movería de golpe.
COMMENT ON COLUMN vacacion.estado IS
  'SOLICITADA (la pidió el colaborador) → APROBADA (la jefatura dio el visto, quedan reservados) → CONFIRMADA (RRHH verificó el disfrute; es la única que cuenta como gozada). RECHAZADA no reserva nada.';

-- El saldo se consulta por empleado y estado; el calendario del equipo, por fecha.
CREATE INDEX idx_vacacion_estado ON vacacion (empresa_id, estado) WHERE NOT anulada;
CREATE INDEX idx_vacacion_fecha ON vacacion (empresa_id, fecha_inicio) WHERE NOT anulada;

-- ---------------------------------------------------------------------------
-- 4. Los permisos
-- ---------------------------------------------------------------------------
-- Separan exactamente lo que pidió el Director: quien SOLICITA, quien APRUEBA y quien ALIMENTA.
-- El tercero ya existe y no se toca (`rrhh.ausencias`, el de RRHH).

INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('rrhh.vacaciones.mias',
     'RRHH / Nómina',
     'Pedir mis vacaciones',
     'Ver el saldo propio y solicitar días. No abre ninguna ficha, salario ni dato de otro empleado.'),
    ('rrhh.vacaciones.equipo',
     'RRHH / Nómina',
     'Aprobar vacaciones del equipo',
     'Ver el calendario y el saldo de días de su departamento y aprobar o rechazar. No abre salarios, deducciones, finiquitos ni incapacidades.')
ON CONFLICT (codigo) DO NOTHING;

-- `rrhh.vacaciones.mias` va a TODOS los roles: pedir vacaciones es algo que hace cualquier
-- persona que entre al sistema, y el alcance ya lo cierra el dato (sin empleado ligado, no ve
-- nada). Negárselo a alguien por su rol sería negarle un derecho laboral por un detalle técnico.
--
-- Se siembra recorriendo las parejas (empresa, rol) QUE YA EXISTEN en `rol_permiso`, y no la
-- tabla `rol`: los roles base son globales y su `empresa_id` es NULL, mientras que `rol_permiso`
-- lo exige. Partir de las parejas existentes es además lo que garantiza que el permiso aterrice en
-- la empresa correcta de cada rol.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
CROSS JOIN permiso p
WHERE p.codigo = 'rrhh.vacaciones.mias'
ON CONFLICT DO NOTHING;

-- `rrhh.vacaciones.equipo` NO se asigna a nadie, a propósito. Quién puede aprobarle los días
-- libres a otra persona es una decisión del Director, nombre por nombre, no un efecto colateral de
-- una migración. Además, sin una fila en `rrhh_jefatura` el permiso no abre nada: el alcance lo
-- pone el servidor y un alcance vacío cierra.

COMMIT;
