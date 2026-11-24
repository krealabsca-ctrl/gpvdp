-- Volver las prioridades al código.
--
-- OJO: si alguien creó prioridades nuevas y las usó, esas facturas quedan con un código que el
-- CHECK de abajo ya no admite. Por eso se limpian ANTES: se les quita la prioridad a las que
-- llevan una que no sea 'A' ni 'AA'. Es pérdida de información, y es la razón por la que esta
-- bajada solo tiene sentido inmediatamente después de la subida.
UPDATE documento_cxp
   SET prioridad = ''
 WHERE COALESCE(prioridad, '') NOT IN ('', 'A', 'AA');

ALTER TABLE documento_cxp
    ADD CONSTRAINT documento_cxp_prioridad_check
    CHECK (prioridad = ANY (ARRAY[''::text, 'A'::text, 'AA'::text]));

DROP TABLE IF EXISTS prioridad_cxp;
