-- Vuelve a mandar al área TODA factura de proveedor nuevo, sin importar el monto.
--
-- Al quitar el parámetro, la regla lo lee como COALESCE(..., 0) y el piso desaparece: es
-- exactamente el comportamiento anterior. No se recalcula nada ya decidido (el veredicto se evalúa
-- al revisar).
DELETE FROM cxp_parametro WHERE clave = 'VALIDACION_PROVEEDOR_NUEVO_PISO_MONTO';
