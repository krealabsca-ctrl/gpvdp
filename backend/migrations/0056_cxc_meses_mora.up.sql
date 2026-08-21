-- ============================================================================
-- CxC · La regla de suspensión son 18 MESES de mora, no 18 cuotas
-- ----------------------------------------------------------------------------
-- Corrección de la 0055. El usuario precisó: «la lógica son 18 meses, o su
-- equivalencia». No es lo mismo:
--
--     Mensual      1 cuota  = 1 mes    ⇒ 18 meses son 18 cuotas
--     Quincenal    1 cuota  = 0,5 mes  ⇒ 18 meses son 36 cuotas
--     Trimestral   1 cuota  = 3 meses  ⇒ 18 meses son 6 cuotas
--     Anual        1 cuota  = 12 meses ⇒ 18 meses son 1,5 cuotas
--
-- Contar cuotas habría suspendido a un quincenal con 9 meses de atraso (la mitad
-- de lo que manda la regla) y habría dejado a un anual acumular 18 años.
--
-- Por eso la medida es el TIEMPO que cubren los cargos vencidos sin pagar, y la
-- equivalencia sale del ciclo de la modalidad que ya está en el catálogo
-- (`cxc_modalidad.meses_ciclo` y `.quincenal`): no hay que preguntar nada nuevo.
-- Las cuotas se siguen mostrando porque son el hecho concreto («debe 36 cuotas»),
-- pero la que decide es la equivalencia en meses.
-- ============================================================================

UPDATE cxc_parametro
SET clave = 'MESES_PARA_SUSPENDER',
    descripcion = 'Meses de mora acumulados (o su equivalencia en cuotas según la modalidad) a partir de los cuales el contrato queda listo para suspender el servicio',
    actualizado_en = now()
WHERE clave = 'CUOTAS_PARA_SUSPENDER';

-- Si alguna empresa no lo tenía, se siembra.
INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, 'MESES_PARA_SUSPENDER', '18',
       'Meses de mora acumulados (o su equivalencia en cuotas según la modalidad) a partir de los cuales el contrato queda listo para suspender el servicio'
FROM empresa e
ON CONFLICT DO NOTHING;

-- La foto de la suspensión guarda las dos medidas: los meses (la regla) y las
-- cuotas (el hecho). Sin los meses no se podría explicar después por qué se
-- suspendió a un quincenal con 36 cuotas y no a los 18.
ALTER TABLE cxc_suspension
    ADD COLUMN meses_mora numeric(6, 1) NOT NULL DEFAULT 0;
