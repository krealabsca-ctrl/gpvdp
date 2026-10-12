-- Un movimiento bancario puede pagar UNA factura o UNA remesa (varias facturas de un proveedor).
--
-- `documento_cxp_id` existe desde la 0043 y solo sabe apuntar a una factura. Con el pago agrupado
-- eso deja de alcanzar: enlazar el débito a la primera de las cinco sería escribir que esa factura
-- se pagó con ₡400.000 cuando costó ₡100.000, y las otras cuatro quedarían abiertas.
--
-- Las dos columnas conviven y son excluyentes: un movimiento es un pago de factura, o de remesa, o
-- de nada. El CHECK lo hace cumplir en la base, no por convención, porque un movimiento con las dos
-- se contaría dos veces en cualquier reporte que cruce Bancos con CxP.
ALTER TABLE movimiento_bancario
    ADD COLUMN IF NOT EXISTS remesa_id uuid REFERENCES remesa_pago(id);

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'movimiento_pago_unico_check') THEN
        ALTER TABLE movimiento_bancario
            ADD CONSTRAINT movimiento_pago_unico_check
            CHECK (documento_cxp_id IS NULL OR remesa_id IS NULL);
    END IF;
END $$;

-- Único parcial, igual que el de documento_cxp_id: una remesa se paga UNA vez. Si dos movimientos
-- reclamaran la misma, el segundo barrido volvería a cerrar facturas ya cerradas y el reporte
-- contaría el pago dos veces.
CREATE UNIQUE INDEX IF NOT EXISTS idx_mov_remesa ON movimiento_bancario (remesa_id)
    WHERE remesa_id IS NOT NULL;
