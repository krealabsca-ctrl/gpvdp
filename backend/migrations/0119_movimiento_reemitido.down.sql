-- Deshacer el enlace de reemisión.
--
-- Los movimientos que quedaron fuera de los libros por un reemplazo NO se devuelven acá: salieron
-- por `incluido = false` con su motivo y su autor, que es la puerta de la mig 0093 y existe
-- independientemente de esto. Devolverlos sería meter de vuelta plata que alguien decidió que
-- estaba repetida, y sin el enlace ya no habría forma de volver a sacarla sabiendo por qué.

ALTER TABLE movimiento_bancario
    DROP CONSTRAINT IF EXISTS ck_mov_reemplazo_completo,
    DROP CONSTRAINT IF EXISTS ck_mov_reemplazo_no_propio;

DROP INDEX IF EXISTS idx_mov_reemplazo_empresa;
DROP INDEX IF EXISTS ux_mov_reemplaza_a;

ALTER TABLE movimiento_bancario
    DROP COLUMN IF EXISTS reemplazo_motivo,
    DROP COLUMN IF EXISTS reemplazo_por,
    DROP COLUMN IF EXISTS reemplazo_en,
    DROP COLUMN IF EXISTS reemplaza_a;
