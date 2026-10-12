-- Se cae la tabla entera: sus filas son un hecho del sistema ANTERIOR, no del ERP. Ningún
-- movimiento, factura ni cargo depende de ellas —el sello se deriva de esta tabla, no se copia a
-- `movimiento_bancario`—, así que quitarla devuelve la base al estado exacto previo.
DROP TABLE IF EXISTS pago_aplicado_externo;
