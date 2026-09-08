-- Departamento y sede como DIMENSIONES del gasto, y presupuesto por departamento.
--
-- Pedido del usuario (2026-08-20): «cómo vinculo esto a un departamento, que también pueda verlo
-- como presupuesto, y así medir si un departamento está gastando de más — ¿cómo lo hacen las
-- empresas grandes para no hacer reprocesos?».
--
-- ── LA DECISIÓN DE FONDO: DIMENSIONES ORTOGONALES ─────────────────────────────
--
-- La partida (Concepto › Clasificación) dice QUÉ se gastó. El departamento dice QUIÉN lo gastó y la
-- sede DÓNDE. Son tres cosas independientes del mismo movimiento, y por eso son tres campos y no un
-- nombre combinado.
--
-- El reproceso que esto evita ya había empezado: hay 22 clasificaciones «Caja Chica - X» donde el
-- lugar o el área está escrito DENTRO del nombre de la partida (medido: 15 son sedes, 5 áreas, 1 una
-- persona y 1 un concepto de gasto). Por ese camino el catálogo se multiplica —161 partidas × 14
-- sedes— y se vuelve imposible contestar «¿cuánto gastamos en combustible en total?» o «¿cuánto
-- gastó Logística en todo?», porque cada respuesta queda repartida entre varios nombres.
--
-- ── LA DECISIÓN QUE EVITA TECLEAR: EL DEFAULT VIVE EN LA PARTIDA ──────────────
--
-- `clasificacion.departamento_id` / `sede_id` es el valor POR DEFECTO de esa partida, y
-- `movimiento_bancario.departamento_id` / `sede_id` es solo la EXCEPCIÓN de un movimiento puntual.
-- El valor efectivo se resuelve al LEER (ver `sqlDepartamentoEfectivo` en dimensiones.go), no se
-- copia al movimiento.
--
-- Eso es lo que hace que no haya reproceso: poner el default en una partida corrige al instante
-- toda su historia —miles de movimientos— sin reclasificar ni volver a importar nada. Si en vez de
-- resolver al leer se guardara una copia en cada movimiento, cambiar un default obligaría a un
-- barrido de actualización, y ese barrido es exactamente el trabajo que el usuario quiere evitar.
--
-- Las dos columnas son NULLABLE a propósito: «sin asignar» es un estado legítimo y visible, no un
-- error. Un movimiento de tesorería (traslado, overnight) no tiene departamento y nunca lo va a
-- tener.

-- ── Sede ─────────────────────────────────────────────────────────────────────
-- Misma forma que `departamento` (migración 0026) para que las dos dimensiones se comporten igual:
-- catálogo por empresa, administrable, con baja lógica y orden de presentación.
--
-- NOTA sobre `cxc_sede`: existe otra tabla de sedes, del módulo de Cobros, que es la frontera de
-- cartera del operador y trae `razon_social`/`plaza` pegados del archivo de origen. Hoy tiene UNA
-- fila, y es de una prueba E2E. Son conceptualmente lo mismo (un lugar físico del negocio) y
-- conviene unificarlas, pero eso toca las claves foráneas de CxC y no se hace de paso: queda dicho
-- acá para que nadie asuma que ya está resuelto.
CREATE TABLE IF NOT EXISTS sede (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    nombre         text NOT NULL,
    codigo         text,
    activo         boolean NOT NULL DEFAULT true,
    orden          integer NOT NULL DEFAULT 0,
    creado_en      timestamptz NOT NULL DEFAULT now(),
    actualizado_en timestamptz NOT NULL DEFAULT now(),
    UNIQUE (empresa_id, nombre)
);

CREATE INDEX IF NOT EXISTS idx_sede_empresa ON sede (empresa_id);

COMMENT ON TABLE sede IS
    'Lugar físico del negocio (sucursal, camposanto, plaza). Dimensión del gasto, independiente del departamento: el departamento dice QUIÉN gastó y la sede DÓNDE.';

-- ── El default por partida ───────────────────────────────────────────────────
ALTER TABLE clasificacion
    ADD COLUMN IF NOT EXISTS departamento_id uuid REFERENCES departamento(id),
    ADD COLUMN IF NOT EXISTS sede_id         uuid REFERENCES sede(id);

COMMENT ON COLUMN clasificacion.departamento_id IS
    'Departamento POR DEFECTO de esta partida. Se resuelve al leer, no se copia al movimiento: ponerlo acá corrige toda la historia de la partida sin reprocesar nada.';
COMMENT ON COLUMN clasificacion.sede_id IS
    'Sede por defecto de esta partida. Mismo criterio que departamento_id.';

-- ── La excepción por movimiento ──────────────────────────────────────────────
ALTER TABLE movimiento_bancario
    ADD COLUMN IF NOT EXISTS departamento_id uuid REFERENCES departamento(id),
    ADD COLUMN IF NOT EXISTS sede_id         uuid REFERENCES sede(id);

COMMENT ON COLUMN movimiento_bancario.departamento_id IS
    'EXCEPCIÓN: departamento de este movimiento en particular, cuando no es el de su partida. NULL = se usa el de la factura de CxP enlazada o el de la partida (ver sqlDepartamentoEfectivo).';
COMMENT ON COLUMN movimiento_bancario.sede_id IS
    'Excepción de sede para este movimiento. Mismo criterio que departamento_id.';

-- Los índices son parciales: la enorme mayoría de los movimientos NO va a tener excepción, y un
-- índice sobre miles de NULL no sirve para nada.
CREATE INDEX IF NOT EXISTS idx_mov_departamento ON movimiento_bancario (empresa_id, departamento_id)
    WHERE departamento_id IS NOT NULL;
CREATE INDEX IF NOT EXISTS idx_mov_sede ON movimiento_bancario (empresa_id, sede_id)
    WHERE sede_id IS NOT NULL;

-- ── Presupuesto por departamento y mes ───────────────────────────────────────
-- Decisión del usuario: se empieza por departamento × mes (≈14 líneas por mes), no por
-- departamento × partida (cientos). Bajar a partida después NO obliga a rehacer esto: se agrega una
-- tabla hermana o una columna nullable de clasificación, y lo cargado sigue siendo válido como el
-- total del departamento.
CREATE TABLE IF NOT EXISTS presupuesto_departamento (
    id              uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id      uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    departamento_id uuid NOT NULL REFERENCES departamento(id),
    -- Período YYYY-MM. Texto y no date porque un presupuesto es de un MES, no de un día, y así la
    -- clave única y las comparaciones son directas contra `to_char(fecha,'YYYY-MM')`.
    periodo         text NOT NULL CHECK (periodo ~ '^\d{4}-\d{2}$'),
    monto_crc       numeric(16,2) NOT NULL CHECK (monto_crc >= 0),
    nota            text,
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now(),
    creado_por      uuid REFERENCES usuario(id),
    UNIQUE (empresa_id, departamento_id, periodo)
);

CREATE INDEX IF NOT EXISTS idx_presupuesto_empresa_periodo
    ON presupuesto_departamento (empresa_id, periodo);

COMMENT ON TABLE presupuesto_departamento IS
    'Monto autorizado por departamento y mes. Se compara contra el gasto real derivado de los movimientos; el presupuesto NO se descuenta ni se consume, se compara.';
COMMENT ON COLUMN presupuesto_departamento.monto_crc IS
    'Monto del mes en colones. CHECK >= 0: un presupuesto negativo no significa nada y sería un error de captura.';
