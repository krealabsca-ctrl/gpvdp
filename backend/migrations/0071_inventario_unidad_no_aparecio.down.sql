-- Volver atrás obliga a reclasificar: las unidades que el conteo no encontró pasan a DANADA,
-- que es lo que el estado anterior permitía.
UPDATE inv_unidad SET estado = 'DANADA' WHERE estado = 'NO_APARECIO';

ALTER TABLE inv_unidad DROP CONSTRAINT IF EXISTS inv_unidad_estado_check;

ALTER TABLE inv_unidad
    ADD CONSTRAINT inv_unidad_estado_check
    CHECK (estado IN ('DISPONIBLE','RESERVADA','EN_TRANSITO','EXHIBICION',
                      'USADA','DANADA','DEVUELTA'));
