-- Revierte la reversa de cargas.
--
-- ⚠ LO QUE ESTE DOWN **NO** HACE, A PROPÓSITO: no vuelve a incluir los movimientos que alguna
--   reversa excluyó. Bajar una migración es deshacer una FUNCIÓN, no deshacer una DECISIÓN
--   financiera: re-incluirlos metería de vuelta a los libros la plata duplicada que el Director
--   sacó a mano, en silencio y sin que nadie apretara nada. Los movimientos quedan con
--   `incluido = false`, que es lo que el resto del sistema ya respetaba antes de esta migración.
--   Lo que se pierde al bajar es la MARCA de quién los excluyó, así que deshacer una reversa
--   después de bajar y volver a subir ya no es posible: hay que re-importar el archivo.
--
-- El motivo y la fecha de cada reversa sobreviven en `auditoria_evento` (tabla inmutable): esas
-- columnas se van, el evento no.

-- 1. La marca en los movimientos (primero el check, que la nombra).
ALTER TABLE movimiento_bancario DROP CONSTRAINT IF EXISTS mov_reversa_implica_excluido;
DROP INDEX IF EXISTS idx_mov_excluido_por_reversa;
ALTER TABLE movimiento_bancario DROP COLUMN IF EXISTS excluido_por_reversa;

-- 2. El permiso (los grants primero: `rol_permiso` los sujeta por FK).
DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'bancos.revertir_importacion');
DELETE FROM permiso WHERE codigo = 'bancos.revertir_importacion';

-- 3. Las cargas que quedaron en REVERTIDA vuelven al estado desde el que se revirtieron: el CHECK
--    viejo no conoce ese valor y las dejaría fuera de la tabla. Se hace ANTES de soltar las
--    columnas, y con el check de coherencia ya fuera del camino.
ALTER TABLE importacion DROP CONSTRAINT IF EXISTS importacion_reversa_coherente;
UPDATE importacion SET estado = COALESCE(NULLIF(estado_previo, ''), 'CONFIRMADA') WHERE estado = 'REVERTIDA';

ALTER TABLE importacion
    DROP COLUMN IF EXISTS estado_previo,
    DROP COLUMN IF EXISTS motivo_reversa,
    DROP COLUMN IF EXISTS revertida_por,
    DROP COLUMN IF EXISTS revertida_en;

ALTER TABLE importacion DROP CONSTRAINT IF EXISTS importacion_estado_check;
ALTER TABLE importacion ADD CONSTRAINT importacion_estado_check
    CHECK (estado IN ('CARGADA', 'PREVISUALIZADA', 'CONFIRMADA', 'CERRADA'));
