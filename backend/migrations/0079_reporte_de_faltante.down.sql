-- Revierte el aviso de faltante.
--
-- Los avisos que NO tienen movimiento se borran primero: son exactamente los que no pueden existir
-- con la columna obligatoria de vuelta. Los que sí lo tienen se conservan.
DROP INDEX IF EXISTS uq_faltante_abierto_por_usuario;

ALTER TABLE movimiento_reporte_segmentacion
  DROP CONSTRAINT IF EXISTS reporte_tiene_de_que_habla;

DELETE FROM movimiento_reporte_segmentacion WHERE movimiento_id IS NULL;

ALTER TABLE movimiento_reporte_segmentacion
  DROP COLUMN IF EXISTS referencia,
  DROP COLUMN IF EXISTS monto_esperado,
  DROP COLUMN IF EXISTS fecha_esperada,
  ALTER COLUMN movimiento_id SET NOT NULL;
