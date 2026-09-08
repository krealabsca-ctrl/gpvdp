-- «Proveedor nuevo» ahora exige que la factura sea MATERIAL.
--
-- ── LO QUE ESTABA PASANDO (medido el 2026-09-03, Valle de Paz) ─────────────
--
-- Estrenar proveedor mandaba la factura al área sin importar el monto. Resultado real:
--
--   · 437 facturas (44 % de TODAS las validaciones de área) por ₡9,3M en total
--   · mediana ₡5.650, mínimo ₡10, máximo ₡235.628
--   · ₡9,3M es el 1 % del dinero que pasa por validación (₡854,7M lo cubre el umbral por MONTO)
--
-- Es decir: casi la mitad del trabajo de validación de los departamentos protegía el 1 % del
-- dinero. Eso es lo que hacía sentir que las áreas validan todo el día, cuando el 78 % de las
-- facturas ya pasaba sin visto bueno de nadie.
--
-- ── POR QUÉ UN PISO PROPIO Y NO EL UMBRAL GENERAL ──────────────────────────
--
-- El umbral general (VALIDACION_UMBRAL_MONTO = ₡250.000) se evalúa ANTES: toda factura que lo
-- supera ya sale como 'MONTO'. Si «proveedor nuevo» exigiera ese mismo valor, la rama quedaría
-- muerta. Por eso lleva su propio piso, más bajo.
--
-- ── EL VALOR ───────────────────────────────────────────────────────────────
--
-- ₡25.000 libera 348 de las 437 (80 %) y conserva ₡6,8M de los ₡9,3M (73 %) bajo revisión. Es
-- editable desde CxP › Validación por riesgo: con ₡50.000 se liberan 382 y quedan ₡5,7M.
--
-- No se recalcula nada de lo ya decidido: el veredicto se evalúa al REVISAR, y esas 437 facturas
-- están en RECIBIDO. La regla nueva aplica cuando Contabilidad las revise.

INSERT INTO cxp_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, 'VALIDACION_PROVEEDOR_NUEVO_PISO_MONTO', '25000',
       'Monto mínimo para que la factura de un proveedor nuevo requiera validación del área. Por debajo, estrenar proveedor no manda la factura al departamento.'
FROM empresa e
ON CONFLICT (empresa_id, clave) DO NOTHING;
