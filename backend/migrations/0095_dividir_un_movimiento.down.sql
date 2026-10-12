-- Revierte la división de movimientos.
--
-- ANTES DE SOLTAR LAS COLUMNAS SE REARMAN LOS PADRES: a cada uno se le devuelve el monto de sus
-- hijos y los hijos se sacan de los libros. Sin eso, al desaparecer `dividido_de` los hijos
-- quedarían como movimientos sueltos con el mismo consecutivo y la misma descripción que su padre,
-- y la plata del mes aparecería contada de más sin que nada diga por qué.
--
-- Los hijos NO se borran —en tablas financieras no hay borrado físico—: quedan con
-- `incluido = false`, que es la misma puerta de la reversa y de la exclusión a mano.

BEGIN;

-- 1. Devolverle a cada padre lo que se le había apartado.
WITH partes AS (
    SELECT dividido_de AS padre_id,
           SUM(COALESCE(debito, 0))  AS debito,
           SUM(COALESCE(credito, 0)) AS credito,
           SUM(COALESCE(monto_crc, 0)) AS monto_crc
    FROM movimiento_bancario
    WHERE dividido_de IS NOT NULL AND incluido
    GROUP BY dividido_de
)
UPDATE movimiento_bancario m
   SET debito    = COALESCE(m.debito, 0)  + p.debito,
       credito   = COALESCE(m.credito, 0) + p.credito,
       monto_crc = COALESCE(m.monto_crc, 0) + p.monto_crc,
       actualizado_en = now()
FROM partes p
WHERE m.id = p.padre_id;

-- 2. Sacar los hijos de los libros.
UPDATE movimiento_bancario
   SET incluido = false, actualizado_en = now()
 WHERE dividido_de IS NOT NULL;

DROP INDEX IF EXISTS idx_mov_dividido_de;

ALTER TABLE movimiento_bancario
    DROP COLUMN IF EXISTS dividido_de,
    DROP COLUMN IF EXISTS dividido_en,
    DROP COLUMN IF EXISTS dividido_por,
    DROP COLUMN IF EXISTS dividido_motivo,
    DROP COLUMN IF EXISTS monto_antes_de_dividir;

DELETE FROM permiso WHERE codigo = 'bancos.dividir_movimiento';

COMMIT;
