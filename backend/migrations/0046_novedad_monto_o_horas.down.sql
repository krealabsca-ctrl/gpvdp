ALTER TABLE corrida_novedad DROP CONSTRAINT IF EXISTS corrida_novedad_monto_o_cantidad;
ALTER TABLE corrida_novedad ADD CONSTRAINT corrida_novedad_monto_check CHECK (monto > 0);
