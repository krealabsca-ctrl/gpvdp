-- Revierte la 0105.
--
-- Se borra la tabla entera, así que se pierden los pagos parciales registrados y el neto de esas
-- facturas vuelve a ser el total: lo que el banco ya debitó dejaría de restarse y la factura
-- pediría pagarse completa. Por eso esta reversa solo tiene sentido si la tabla está vacía — que
-- es el caso al revertir una migración recién aplicada.
DROP TABLE IF EXISTS pago_parcial;

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'cxp.cobrar_saldo');
DELETE FROM permiso WHERE codigo = 'cxp.cobrar_saldo';
