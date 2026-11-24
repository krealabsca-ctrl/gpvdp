-- Quitar el cronograma de pago.
--
-- SE PIERDEN DECISIONES, no solo columnas: de qué cuenta sale cada pago, qué se marcó para pagar
-- por fuera del cronograma (y quién lo decidió), qué cuentas son reserva y con qué tipo de cambio
-- se armó cada cuadro. Nada de eso se puede recuperar de otra tabla.

ALTER TABLE lote_pago
    DROP CONSTRAINT IF EXISTS ck_lote_tc_cronograma_positivo;
ALTER TABLE lote_pago
    DROP COLUMN IF EXISTS tc_cronograma;

DROP INDEX IF EXISTS idx_docxp_por_fuera;
DROP INDEX IF EXISTS idx_docxp_cuenta_pago;

ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS por_fuera_en,
    DROP COLUMN IF EXISTS por_fuera_por,
    DROP COLUMN IF EXISTS por_fuera,
    DROP COLUMN IF EXISTS cuenta_pago_id;

ALTER TABLE cuenta_bancaria
    DROP COLUMN IF EXISTS es_reserva;
