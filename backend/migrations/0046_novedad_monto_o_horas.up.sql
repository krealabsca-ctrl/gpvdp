-- Una novedad vale por su MONTO o por sus HORAS, nunca por ninguno de los dos.
--
-- El CHECK original exigía monto > 0, lo que impedía guardar una novedad de horas extra (su
-- monto lo deriva el motor con el salario vigente, así que nace en cero). Se reemplaza por la
-- regla completa: o trae monto, o trae horas — nunca una novedad vacía.
ALTER TABLE corrida_novedad DROP CONSTRAINT IF EXISTS corrida_novedad_monto_check;

ALTER TABLE corrida_novedad
    ADD CONSTRAINT corrida_novedad_monto_o_cantidad CHECK (
        monto > 0 OR (monto = 0 AND cantidad IS NOT NULL AND cantidad > 0)
    );
