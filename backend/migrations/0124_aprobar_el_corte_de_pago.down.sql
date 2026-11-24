-- Revierte la aprobación del corte de pago.
--
-- Los lotes que quedaron EN_APROBACION o APROBADO vuelven a ABIERTO: son los dos estados que
-- dejan de existir, y dejarlos puestos haría que el CHECK viejo rechazara la fila. Un corte
-- aprobado que vuelve a abierto es exactamente lo que había antes de esta migración.

UPDATE lote_pago SET estado = 'ABIERTO' WHERE estado IN ('EN_APROBACION', 'APROBADO');

ALTER TABLE lote_pago DROP CONSTRAINT IF EXISTS lote_pago_estado_check;
ALTER TABLE lote_pago ADD CONSTRAINT lote_pago_estado_check
    CHECK (estado IN ('ABIERTO', 'CERRADO'));

ALTER TABLE lote_pago
    DROP COLUMN IF EXISTS enviado_aprobacion_por,
    DROP COLUMN IF EXISTS enviado_aprobacion_en,
    DROP COLUMN IF EXISTS aprobado_por,
    DROP COLUMN IF EXISTS aprobado_en,
    DROP COLUMN IF EXISTS devuelto_por,
    DROP COLUMN IF EXISTS devuelto_en,
    DROP COLUMN IF EXISTS devolucion_motivo;

DROP INDEX IF EXISTS idx_docxp_excluido_lote;

ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS excluido_lote_por,
    DROP COLUMN IF EXISTS excluido_lote_en,
    DROP COLUMN IF EXISTS excluido_lote_motivo,
    DROP COLUMN IF EXISTS excluido_lote_numero;

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'cxp.aprobar_lote');
DELETE FROM permiso WHERE codigo = 'cxp.aprobar_lote';
