-- La reversa LIBERA la línea: una carga revertida deja de ocupar su huella.
--
-- ── EL PROBLEMA ─────────────────────────────────────────────────────────────
--
-- `natural_key` lleva la cuenta adentro y es única por empresa. Cuando una carga se revierte
-- (mig 0085) sus movimientos NO se borran —regla del proyecto para tablas financieras— así que
-- siguen ocupando la huella. Consecuencia medida: corregir el archivo y volver a subirlo A LA
-- MISMA CUENTA inserta CERO filas, porque el `ON CONFLICT ... DO NOTHING` las descarta una por
-- una, y la pantalla dice «0 insertados» sin explicar por qué. El usuario concluye que el
-- sistema perdió su archivo.
--
-- ── LA DECISIÓN DEL DIRECTOR FINANCIERO ─────────────────────────────────────
--
-- Se le plantearon las dos salidas y eligió: «que la reversa libere la línea», a cambio de que
-- «Deshacer la reversa» se bloquee cuando esas líneas ya se volvieron a importar. Es la que deja
-- el camino feliz sin fricción (corregir y volver a subir, que es lo que se hace de verdad) y
-- pone el freno en el camino raro (deshacer algo que ya se rehízo).
--
-- ── POR QUÉ UN ÍNDICE ÚNICO PARCIAL Y NO OTRA COSA ──────────────────────────
--
-- Cambiar el Go no alcanza: aunque `NaturalKeysExistentes` deje de contar las revertidas, el
-- INSERT choca igual contra el UNIQUE de la tabla y `DO NOTHING` se come la fila EN SILENCIO —el
-- peor de los finales, porque el conteo de insertados queda corto sin un solo error visible. La
-- unicidad tiene que dejar de aplicar a las filas revertidas EN LA BASE.
--
-- Las alternativas que se descartaron:
--   · meter `excluido_por_reversa` dentro de la clave: dos filas revertidas idénticas chocarían
--     entre ellas, que es justo lo que no importa;
--   · borrar la fila revertida: prohibido, y además tira a la basura la clasificación hecha;
--   · quitar la unicidad del todo: se pierde la idempotencia de reimportar, que es la razón por
--     la que existe la huella.
--
-- El índice parcial dice exactamente lo que queremos: entre las filas QUE CUENTAN, la huella es
-- única. Entre las revertidas no hay regla, porque no son plata.
--
-- `ON CONFLICT` deja de inferir solo: hay que repetirle el predicado
-- (`ON CONFLICT (empresa_id, natural_key) WHERE NOT excluido_por_reversa`), y eso está hecho en
-- repository.go junto a este comentario.

ALTER TABLE movimiento_bancario
    DROP CONSTRAINT movimiento_bancario_empresa_id_natural_key_key;

CREATE UNIQUE INDEX ux_mov_natural_key_vigente
    ON movimiento_bancario (empresa_id, natural_key)
    WHERE NOT excluido_por_reversa;
