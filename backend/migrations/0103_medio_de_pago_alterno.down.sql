DROP INDEX IF EXISTS idx_doc_medio_pago;

ALTER TABLE documento_cxp
    DROP CONSTRAINT IF EXISTS documento_medio_pago_con_motivo_check,
    DROP CONSTRAINT IF EXISTS documento_vale_solo_caja_chica_check,
    DROP CONSTRAINT IF EXISTS documento_medio_pago_check;

ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS medio_pago_por,
    DROP COLUMN IF EXISTS medio_pago_en,
    DROP COLUMN IF EXISTS medio_pago_vale_id,
    DROP COLUMN IF EXISTS medio_pago_motivo,
    DROP COLUMN IF EXISTS medio_pago;
