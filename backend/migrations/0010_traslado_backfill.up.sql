-- Backfill de traslados (Fase 1, pulido): los movimientos ya clasificados a un
-- concepto de traslado/overnight deben quedar marcados es_traslado = true, para
-- excluirlos del EBITDA y del cuadre de forma coherente con su clasificación.
-- (A partir de ahora, clasificar a esos conceptos setea es_traslado automáticamente.)
UPDATE movimiento_bancario m
SET es_traslado = true, actualizado_en = now()
FROM concepto c
WHERE m.concepto_id = c.id
  AND m.empresa_id = c.empresa_id
  AND (c.nombre ILIKE '%traslado%' OR c.nombre ILIKE '%overnight%')
  AND m.es_traslado = false;
