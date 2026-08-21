-- Separación de catálogos (decisión del DF 2026-07-16): el catálogo bancario tiene
-- conceptos SENSIBLES que contabilidad no debe ver desde CxP. `visible_cxp` controla
-- qué conceptos (y sus clasificaciones) aparecen en el clasificador de gastos de CxP.
ALTER TABLE concepto ADD COLUMN visible_cxp boolean NOT NULL DEFAULT true;

-- Backfill: CxP conserva exactamente lo que YA usa (documentos, gasto default del
-- proveedor o gastos frecuentes); todo lo demás queda como catálogo bancario privado.
UPDATE concepto SET visible_cxp = false
WHERE id NOT IN (
    SELECT concepto_id FROM documento_cxp WHERE concepto_id IS NOT NULL
    UNION
    SELECT gasto_concepto_id FROM proveedor WHERE gasto_concepto_id IS NOT NULL
    UNION
    SELECT concepto_id FROM proveedor_gasto
);
