-- Volver al UNIQUE de tabla. OJO: si alguien ya re-importó las líneas de una carga revertida, hay
-- dos filas con la misma huella y este ALTER FALLA. Es lo correcto: bajar la migración con datos
-- así obligaría a elegir cuál de las dos plata se borra, y eso no lo decide una migración.
DROP INDEX IF EXISTS ux_mov_natural_key_vigente;

ALTER TABLE movimiento_bancario
    ADD CONSTRAINT movimiento_bancario_empresa_id_natural_key_key UNIQUE (empresa_id, natural_key);
