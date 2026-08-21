-- Bancos — corrección del criterio de traslado (reportada por el usuario con evidencia).
--
-- PROBLEMA: `es_traslado` se activaba con solo clasificar el movimiento con un concepto
-- llamado «Traslado» u «Overnight», sin exigir la pata contraria. Y `es_traslado` es lo
-- único que excluye del EBITDA (ver repository_analisis.go), así que un cobro de plan de
-- cliente mal clasificado salía del resultado sin que nada lo delatara. En Valle de Paz eso
-- dejó 155 movimientos marcados como traslado con UNA sola pareja confirmada: ₡555 M fuera
-- del EBITDA sin contraparte que los justifique.
--
-- REGLA CONFIRMADA POR EL DIRECTOR FINANCIERO: los traslados/overnight **emparejados** no
-- cuentan. Un traslado sin su pata contraria es, hasta que se empareje, un movimiento normal.
--
-- Este ajuste devuelve la bandera a su única fuente de verdad: existe un par confirmado.
-- El concepto asignado NO se toca (sigue siendo «Traslados de Fondos» u «Overnight»): lo que
-- cambia es que ya no excluye del EBITDA por sí solo. Al emparejar, la bandera vuelve sola.

UPDATE movimiento_bancario
SET es_traslado = false, actualizado_en = now()
WHERE es_traslado = true AND par_traslado_id IS NULL;
