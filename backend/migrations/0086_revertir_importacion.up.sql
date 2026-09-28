-- Revertir una CARGA de banco entera, desde la pantalla y sin borrar nada.
--
-- ── EL INCIDENTE ────────────────────────────────────────────────────────────
--
-- El 23-set-2026 el archivo «VALLE DE PAZ PROMERICA COLONES.xlsx» se importó en la cuenta
-- EQUIVOCADA (Promerica Colinas): 7 movimientos, ₡10.000.000 en débitos y ₡7.042.727,09 en
-- créditos, gemelos exactos de los que ya estaban bien cargados en Promerica VDP. Esa plata
-- quedó contada DOS VECES. El anti-duplicado no lo atajó porque la cuenta va ADENTRO de la
-- `natural_key`: el mismo archivo en otra cuenta no choca con nada.
--
-- En esa MISMA cuenta equivocada hay otros 5 movimientos de julio, de otra carga, que están
-- BIEN y dos de ellos ya clasificados. La regla que dio el Director es literal:
--
--   «debe permitirme excluir todo lo que duplico sin mal lograr lo que esta bien, por ejm del
--    día 20 hacia atras todo esta bien y el 21 agregue mal los bancos, lo que debo corregir es
--    lo cargado el 21 no lo que ya estaba cargado el 20»
--
-- De ahí sale LA UNIDAD DE LA REVERSA: la CARGA (la importación), no la fecha del movimiento ni
-- la cuenta. Filtrar por fecha se llevaría por delante los de julio; revertir la carga no toca
-- ni una fila que no haya venido en ese archivo.
--
-- Y la segunda mitad de la frase —«la solucion no debe ser por codigo»— es la que obliga a esta
-- migración: hoy `movimiento_bancario.incluido` lo respetan ~20 consultas del sistema pero NO
-- existe un solo camino de producción que lo escriba. La columna no tiene picaporte: la única
-- forma de excluir un duplicado es entrar a Postgres a mano.
--
-- ── LO QUE NO SE HACE ───────────────────────────────────────────────────────
--
-- NO se borra nada. Ni la importación, ni sus movimientos, ni su clasificación. La reversa es
-- `incluido = false` + el estado de la carga + un evento de auditoría, que es la regla del
-- proyecto para tablas financieras. Si mañana el archivo se vuelve a importar en la cuenta
-- correcta, el trabajo de clasificación hecho sobre el equivocado sigue ahí para consultarlo.

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. El estado nuevo de la carga
-- ─────────────────────────────────────────────────────────────────────────────
ALTER TABLE importacion DROP CONSTRAINT importacion_estado_check;
ALTER TABLE importacion ADD CONSTRAINT importacion_estado_check
    CHECK (estado IN ('CARGADA', 'PREVISUALIZADA', 'CONFIRMADA', 'CERRADA', 'REVERTIDA'));

-- `motivo_reversa` es OBLIGATORIO al revertir y por eso vive acá y no solo en la auditoría:
-- dentro de tres meses, quien abra la pantalla y vea que faltan ₡7 millones tiene que leer POR QUÉ
-- en la misma fila, sin ir a buscar el evento.
--
-- `estado_previo` guarda de dónde vino la carga para que deshacer la reversa la devuelva EXACTA.
-- Sin esta columna, deshacer tendría que asumir 'CONFIRMADA' y una carga que nunca se confirmó
-- (una previsualización con 0 movimientos, por ejemplo) quedaría diciendo que sí: una mentira
-- chiquita que después nadie puede desarmar.
ALTER TABLE importacion
    ADD COLUMN revertida_en   timestamptz,
    ADD COLUMN revertida_por  uuid REFERENCES usuario(id),
    ADD COLUMN motivo_reversa text NOT NULL DEFAULT '',
    ADD COLUMN estado_previo  text;

-- Las tres columnas de arriba y el estado tienen que contar la MISMA historia. Sin este check, un
-- `UPDATE` a medias deja una carga REVERTIDA sin motivo (y la pantalla no puede explicar el
-- faltante) o una carga viva con fecha de reversa (y la pantalla la pinta como revertida).
ALTER TABLE importacion ADD CONSTRAINT importacion_reversa_coherente CHECK (
    (estado <> 'REVERTIDA' AND revertida_en IS NULL AND revertida_por IS NULL AND motivo_reversa = '')
    OR
    (estado =  'REVERTIDA' AND revertida_en IS NOT NULL AND btrim(motivo_reversa) <> '')
);

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. Qué fila excluyó ESTA reversa
-- ─────────────────────────────────────────────────────────────────────────────
--
-- Deshacer la reversa tiene que devolver a `incluido = true` EXACTAMENTE las filas que la reversa
-- apagó, «ni una más». Con solo `incluido` eso es imposible de saber: un `WHERE NOT incluido`
-- re-incluiría también lo que alguien había excluido antes y por otro motivo, y volvería a meter
-- plata duplicada a los libros justo cuando se está tratando de sacarla. La marca es lo que hace
-- que deshacer sea una reversa y no un barrido.
ALTER TABLE movimiento_bancario
    ADD COLUMN excluido_por_reversa boolean NOT NULL DEFAULT false;

-- Un movimiento marcado por la reversa pero contando plata es la peor de las dos mentiras
-- posibles: la carga figura revertida y el dinero sigue en los totales.
ALTER TABLE movimiento_bancario ADD CONSTRAINT mov_reversa_implica_excluido
    CHECK (NOT excluido_por_reversa OR NOT incluido);

-- Deshacer busca por (importación, marca); el índice parcial es diminuto porque casi ninguna fila
-- lleva la marca.
CREATE INDEX idx_mov_excluido_por_reversa
    ON movimiento_bancario (importacion_id) WHERE excluido_por_reversa;

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. El permiso
-- ─────────────────────────────────────────────────────────────────────────────
--
-- Aparte de `bancos.importar` a propósito: importar es el trabajo diario de la auxiliar; sacar
-- ₡7 millones de los libros no lo es. Decisión del Director Financiero: revertir una carga lo
-- pueden hacer «solo el Admin o el Financiero».
--
-- `critico = true`: el efecto es que deja de contar plata que ya estaba en los estados.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico) VALUES
  ('bancos.revertir_importacion', 'Bancos', 'Revertir una carga del banco',
   'Sacar de los libros TODOS los movimientos que trajo una importación equivocada (y volver a ponerlos), con motivo. No borra nada',
   true)
ON CONFLICT (codigo) DO NOTHING;

-- ── Y ACÁ VA LO QUE SIEMPRE SE OLVIDA ────────────────────────────────────────
--
-- Los permisos NO se resuelven leyendo el catálogo de Go en vivo: están MATERIALIZADOS en
-- `rol_permiso`. Agregar la fila al catálogo de código (internal/rbac/catalogo.go) hace que una
-- empresa NUEVA nazca con el permiso —DIRECTOR_FINANCIERO hereda todo porque su matriz es
-- `codigos()`— pero `rbac.EnsureDefaults` NO reparte permisos nuevos a una empresa que ya tiene su
-- matriz configurada. Hay deriva comprobada: al Director le faltan hoy permisos que la matriz dice
-- que debería tener. Así que la migración siembra ella misma, para las tres empresas que ya existen.
--
-- ADMIN no aparece: tiene bypass en el checker (internal/rbac/catalogo.go, RolAdmin).
--
-- El rol base vive con `empresa_id IS NULL` y se le conceden permisos POR EMPRESA en `rol_permiso`;
-- la condición del OR contempla además un DIRECTOR_FINANCIERO creado a medida dentro de una empresa.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT e.id, r.id, p.id
FROM empresa e
CROSS JOIN rol r
CROSS JOIN permiso p
WHERE r.codigo = 'DIRECTOR_FINANCIERO'
  AND (r.empresa_id IS NULL OR r.empresa_id = e.id)
  AND p.codigo = 'bancos.revertir_importacion'
ON CONFLICT DO NOTHING;
