-- Deshace la remesa de pago.
--
-- El orden importa: primero la columna que apunta a la tabla, después la tabla. Al revés, la FK
-- impide el DROP y la migración queda a medias.
ALTER TABLE documento_cxp DROP COLUMN IF EXISTS remesa_id;
ALTER TABLE lote_pago DROP CONSTRAINT IF EXISTS lote_pago_modo_pago_check;
ALTER TABLE lote_pago DROP COLUMN IF EXISTS modo_pago;
DROP TABLE IF EXISTS remesa_pago;
