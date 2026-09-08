-- El conteo cíclico necesitaba un estado que no existía.
--
-- Cuando una unidad no aparece en el conteo, el cierre la daba de baja como DANADA. Pero «no
-- apareció» no es «se dañó»: la ficha decía «dada de baja por daño» para un cofre que se había
-- usado en un servicio sin registrarlo, o que estaba mal ubicado. El motivo verdadero quedaba
-- solo en el movimiento del libro, y la ficha —que es lo que la gente mira— afirmaba otra cosa.
--
-- Es la misma lección de siempre: un estado nuevo necesita su propio nombre. Reusar el más
-- parecido tapa el hecho que se quería registrar.
ALTER TABLE inv_unidad DROP CONSTRAINT IF EXISTS inv_unidad_estado_check;

ALTER TABLE inv_unidad
    ADD CONSTRAINT inv_unidad_estado_check
    CHECK (estado IN ('DISPONIBLE','RESERVADA','EN_TRANSITO','EXHIBICION',
                      'USADA','DANADA','DEVUELTA','NO_APARECIO'));

COMMENT ON COLUMN inv_unidad.estado IS
    'DISPONIBLE se puede vender · RESERVADA apartada para un servicio · EN_TRANSITO va entre sedes '
    '· EXHIBICION en sala · USADA salió en un servicio · DANADA se dañó · DEVUELTA volvió al '
    'proveedor · NO_APARECIO el conteo no la encontró (el motivo está en el movimiento de baja).';
