-- Revierte la exclusión de movimientos sueltos.
--
-- LOS MOVIMIENTOS EXCLUIDOS A MANO SE VUELVEN A INCLUIR, y es deliberado: al desaparecer las
-- columnas, un movimiento con `incluido = false` y sin `excluido_por_reversa` quedaría fuera de los
-- libros sin que nada diga por qué ni quién lo sacó, y nadie podría volver a meterlo. Es preferible
-- que la plata vuelva a los libros —visible, auditable, y duplicada si lo estaba— a que quede
-- afuera sin rastro.

BEGIN;

UPDATE movimiento_bancario
   SET incluido = true, actualizado_en = now()
 WHERE NOT incluido AND NOT excluido_por_reversa;

DROP INDEX IF EXISTS idx_mov_excluido_a_mano;
DROP INDEX IF EXISTS idx_mov_duplicado;

ALTER TABLE movimiento_bancario
    DROP COLUMN IF EXISTS excluido_motivo,
    DROP COLUMN IF EXISTS excluido_por,
    DROP COLUMN IF EXISTS excluido_en;

DELETE FROM permiso WHERE codigo = 'bancos.excluir_movimiento';

COMMIT;
