-- Consulta de bancos POR SEGMENTO: que un equipo vea solo las partidas que se le asignaron.
--
-- ── EL PROBLEMA ─────────────────────────────────────────────────────────────
--
-- Emergencias, Depósitos y Asociaciones necesitan una sola cosa del banco: saber si el dinero
-- entró. Hoy la única forma de dárselo es `bancos.ver_clasificar`, que muestra los 18 694
-- movimientos de la empresa, o pasarles un Excel armado a mano —que no se puede despublicar, no
-- deja rastro de quién lo abrió y queda viejo el mismo día—.
--
-- ── EL MODELO ───────────────────────────────────────────────────────────────
--
-- Dos capas distintas, y esto es lo que hace que sea seguro:
--
--   · el PERMISO (`bancos.ver_mi_segmento`) dice QUÉ PANTALLA puede abrir;
--   · el ALCANCE (`rol_clasificacion_consulta`) dice CUÁLES FILAS ve ahí.
--
-- El alcance se deriva de la segmentación que ya se hace en el catálogo: si un movimiento se
-- reclasifica, cambia de dueño solo. Y un movimiento sin clasificar no está en ningún alcance, así
-- que no lo ve nadie — que es la presión que mantiene Bancos bien segmentado.
--
-- NO se le da el permiso a ningún rol existente, a propósito. Es una capacidad nueva para los roles
-- de consulta que el negocio va a crear (Cobros Asociaciones, Servicio / Sala, Emergencias); los
-- roles financieros ya leen el módulo por su propio permiso y esta pantalla no les agrega nada.
-- DIRECTOR_FINANCIERO lo recibe igual porque su matriz es «todos los códigos», y ahí es inofensivo:
-- con el alcance vacío la pantalla no muestra ni una fila.

-- 1. El permiso. `critico = false`: es lectura, y de un pedazo.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico) VALUES
  ('bancos.ver_mi_segmento', 'Bancos', 'Consultar mi segmento de bancos',
   'Ver SOLO los créditos de las partidas asignadas al rol, y avisar cuando alguno quedó mal segmentado. No abre ninguna otra pantalla de Bancos',
   false)
ON CONFLICT (codigo) DO NOTHING;

-- 2. El alcance: qué partidas consulta cada rol.
--
-- `empresa_id` va en la tabla aunque se pueda deducir del rol o de la clasificación: es la columna
-- por la que filtra todo el sistema, y deducirla en cada consulta es justo el descuido que abre una
-- fuga entre empresas.
CREATE TABLE IF NOT EXISTS rol_clasificacion_consulta (
  empresa_id       uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
  rol_id           uuid NOT NULL REFERENCES rol(id) ON DELETE CASCADE,
  clasificacion_id uuid NOT NULL REFERENCES clasificacion(id) ON DELETE CASCADE,
  creado_en        timestamptz NOT NULL DEFAULT now(),
  PRIMARY KEY (empresa_id, rol_id, clasificacion_id)
);

-- Los dos sentidos de lectura: «qué ve este rol» (la pantalla del equipo, en cada carga) y «quién
-- ve esta partida» (la columna del catálogo).
CREATE INDEX IF NOT EXISTS idx_rcc_rol ON rol_clasificacion_consulta (empresa_id, rol_id);
CREATE INDEX IF NOT EXISTS idx_rcc_clasificacion ON rol_clasificacion_consulta (empresa_id, clasificacion_id);

-- 3. Los avisos de mala segmentación.
--
-- El equipo no corrige la partida: la señala. Corregir sigue siendo de quien clasifica, en la
-- pantalla donde ya se clasifica.
CREATE TABLE IF NOT EXISTS movimiento_reporte_segmentacion (
  id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
  movimiento_id uuid NOT NULL REFERENCES movimiento_bancario(id),
  usuario_id    uuid NOT NULL REFERENCES usuario(id),
  motivo        text NOT NULL,
  creado_en     timestamptz NOT NULL DEFAULT now(),
  -- Resolución. Nulo = pendiente: el estado se DERIVA de estas columnas, no hay una columna
  -- «estado» que haya que mantener sincronizada.
  resuelto_en   timestamptz,
  resuelto_por  uuid REFERENCES usuario(id),
  resolucion    text CHECK (resolucion IN ('RECLASIFICADO', 'SIN_CAMBIO')),
  respuesta     text,
  -- Las dos columnas de resolución van juntas o no van: un reporte «resuelto» sin quién ni cómo no
  -- sirve para explicarle nada al equipo que preguntó.
  CONSTRAINT reporte_resolucion_completa CHECK (
    (resuelto_en IS NULL AND resuelto_por IS NULL AND resolucion IS NULL)
    OR (resuelto_en IS NOT NULL AND resuelto_por IS NOT NULL AND resolucion IS NOT NULL)
  )
);

-- Un solo reporte ABIERTO por movimiento: si tres personas avisan del mismo, quien clasifica
-- resuelve una vez y no tres. Parcial a propósito — el histórico de reportes resueltos del mismo
-- movimiento sí puede tener varias filas.
CREATE UNIQUE INDEX IF NOT EXISTS uq_reporte_abierto_por_movimiento
  ON movimiento_reporte_segmentacion (movimiento_id) WHERE resuelto_en IS NULL;

CREATE INDEX IF NOT EXISTS idx_reporte_seg_empresa
  ON movimiento_reporte_segmentacion (empresa_id, creado_en DESC);
