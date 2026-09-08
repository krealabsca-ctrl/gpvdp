-- Revertir esto reabre el camino al doble pago: las provisiones de consignación vuelven a ser
-- programables y a entrar en el archivo de pagos. Si hay alguna viva, abortá y resolvela primero
-- (conciliándola o anulándola en CxP) en vez de quitarle el candado.
DO $$
DECLARE n int;
BEGIN
    SELECT count(*) INTO n FROM documento_cxp
     WHERE bloqueado_para_pago AND estado NOT IN ('ANULADO', 'PAGADO', 'CONCILIADO', 'DENEGADO');
    IF n > 0 THEN
        RAISE EXCEPTION 'hay % documento(s) bloqueado(s) para pago todavía vivos: quitar el candado los volvería pagables y se podría pagar dos veces el mismo cofre. Conciliá o anulá esas provisiones antes de revertir.', n;
    END IF;
END $$;

DROP INDEX IF EXISTS idx_documento_cxp_bloqueado;

ALTER TABLE documento_cxp
    DROP COLUMN IF EXISTS bloqueo_motivo,
    DROP COLUMN IF EXISTS bloqueado_para_pago;
