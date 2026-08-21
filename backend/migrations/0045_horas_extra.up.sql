-- Horas extra calculadas (Código de Trabajo, art. 139).
--
-- El concepto «Horas extra» ya existía y se pagaba registrando el MONTO a mano. Eso es donde se
-- cometen los errores: hay que saber el valor de la hora, multiplicarlo por 1,5 y por las horas.
-- Ahora se capturan las HORAS y el monto se deriva.
--
-- El monto NO se guarda en la novedad: se recalcula con el salario vigente cada vez que se
-- recalcula la corrida (misma disciplina que el resto del módulo — derivado, no almacenado).

-- cantidad = horas trabajadas. NULL = novedad de monto directo (comisión, bono, viático), que
-- sigue funcionando igual que antes.
ALTER TABLE corrida_novedad
    ADD COLUMN cantidad numeric(8, 2) CHECK (cantidad IS NULL OR cantidad > 0);

-- Parámetros del año (versionados, como las cargas y los tramos de renta).
--
-- horas_jornada_mes: las horas ordinarias de un mes, para sacar el valor de la hora a partir del
-- salario mensual. 240 = 30 días × 8 horas, que es el uso corriente en Costa Rica. Un divisor MÁS
-- ALTO abarata la hora, así que se acota por CHECK para que nadie lo suba por error.
--
-- factor_hora_extra: 1,5 = «tiempo y medio» del art. 139. El CHECK impide bajarlo del mínimo
-- legal; subirlo sí se permite (hay acuerdos que pagan más).
ALTER TABLE nomina_parametros
    ADD COLUMN horas_jornada_mes numeric(6, 2) NOT NULL DEFAULT 240
        CHECK (horas_jornada_mes BETWEEN 160 AND 260),
    ADD COLUMN factor_hora_extra numeric(4, 2) NOT NULL DEFAULT 1.5
        CHECK (factor_hora_extra >= 1.5);
