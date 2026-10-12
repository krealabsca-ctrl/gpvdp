-- Remesa de pago: un solo pago que cubre VARIAS facturas del mismo proveedor.
--
-- Hasta hoy el corte generaba una transferencia por factura. Medido sobre los 28 lotes reales de
-- Valle de Paz, 9 tenían más de una factura del mismo proveedor: el lote #3 salió con 11
-- transferencias para 8 proveedores y el #8 con 10 para 8. Cada transferencia de más es una
-- comisión, una línea más que cuadrar y un proveedor que recibe tres depósitos sueltos el mismo día.
--
-- POR QUÉ UNA TABLA Y NO UN GROUP BY: la agrupación sí es derivable (lote + proveedor), pero la
-- REFERENCIA que viaja al banco no lo es. Tiene que ser estable en el tiempo —vuelve semanas
-- después en el estado de cuenta— e irrepetible, así que se guarda. El monto también: es lo que
-- SALIÓ del banco, y no puede recalcularse después porque las facturas que cubre pueden cambiar de
-- estado (una se anula, otra se devuelve) y entonces la suma de hoy ya no sería la de aquel día.
CREATE TABLE IF NOT EXISTS remesa_pago (
    id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id   uuid NOT NULL REFERENCES empresa(id),
    lote_id      uuid NOT NULL REFERENCES lote_pago(id),
    proveedor_id uuid NOT NULL REFERENCES proveedor(id),
    -- Consecutivo DENTRO del lote: es la mitad legible de la referencia («REM 0043-07»), lo que el
    -- proveedor y el banco pueden leer sin tener el sistema delante.
    numero       int NOT NULL,
    -- Código interno del pago. Mismo largo que el de factura (12) y prefijo CXPR para que el
    -- lector pueda distinguirlos sin ambigüedad aunque el banco borre el guion (ver reRemesa).
    huella       text NOT NULL,
    -- Lo que se mandó pagar: la suma de los netos de sus facturas AL MOMENTO del corte.
    monto_crc    numeric(16,2) NOT NULL DEFAULT 0,
    creado_en    timestamptz NOT NULL DEFAULT now(),
    -- Un proveedor no puede tener dos remesas en el mismo lote: eso sería volver a partir el pago.
    CONSTRAINT remesa_pago_lote_proveedor_key UNIQUE (empresa_id, lote_id, proveedor_id),
    -- La referencia es la llave del emparejamiento: dos iguales enlazarían el pago equivocado.
    CONSTRAINT remesa_pago_huella_key UNIQUE (empresa_id, huella),
    CONSTRAINT remesa_pago_numero_key UNIQUE (empresa_id, lote_id, numero)
);

CREATE INDEX IF NOT EXISTS idx_remesa_lote ON remesa_pago (empresa_id, lote_id);

-- Qué remesa cubre cada factura. NULL = se paga sola, con su propia referencia (el modo de
-- siempre). No se borra en cascada: una remesa es un hecho del pasado y sus facturas quedan
-- apuntando a ella aunque después pasen a PAGADO o CONCILIADO.
ALTER TABLE documento_cxp
    ADD COLUMN IF NOT EXISTS remesa_id uuid REFERENCES remesa_pago(id);

CREATE INDEX IF NOT EXISTS idx_documento_remesa ON documento_cxp (remesa_id)
    WHERE remesa_id IS NOT NULL;

-- Cómo se generó el lote. Se guarda en el LOTE y no se infiere de si hay remesas, porque un lote
-- por proveedor donde cada proveedor trajo una sola factura no genera agrupación visible y aun así
-- se armó con ese criterio: el reporte y la macro tienen que saberlo para no cambiar de forma.
ALTER TABLE lote_pago
    ADD COLUMN IF NOT EXISTS modo_pago text NOT NULL DEFAULT 'FACTURA';

DO $$
BEGIN
    IF NOT EXISTS (SELECT 1 FROM pg_constraint WHERE conname = 'lote_pago_modo_pago_check') THEN
        ALTER TABLE lote_pago
            ADD CONSTRAINT lote_pago_modo_pago_check CHECK (modo_pago IN ('FACTURA', 'PROVEEDOR'));
    END IF;
END $$;
