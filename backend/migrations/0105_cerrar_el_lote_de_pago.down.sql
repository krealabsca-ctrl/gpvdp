-- Revierte la 0104.
--
-- Los lotes que se hayan cerrado vuelven a ABIERTO: sin las columnas del cierre, un lote en
-- 'CERRADO' quedaría trancado para siempre —nada podría reabrirlo— y eso es peor que perder el
-- sello. La huella de quién cerró y por qué se reabrió queda en auditoria_evento, que no se toca.
UPDATE lote_pago SET estado = 'ABIERTO' WHERE estado = 'CERRADO';

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'cxp.reabrir_lote');
DELETE FROM permiso WHERE codigo = 'cxp.reabrir_lote';

ALTER TABLE lote_pago
    DROP COLUMN IF EXISTS cerrado_por,
    DROP COLUMN IF EXISTS cerrado_en,
    DROP COLUMN IF EXISTS reabierto_por,
    DROP COLUMN IF EXISTS reabierto_en,
    DROP COLUMN IF EXISTS reapertura_motivo;
