-- Backfill de plazos de pago del proveedor a partir de sus propias facturas importadas de
-- Factun: si sus facturas dicen "Condición: Crédito" y traen vencimiento, el plazo típico es
-- la moda de (vencimiento − emisión). Solo toca proveedores aún sin condiciones (CONTADO/0);
-- lo fijado a mano no se pisa.

UPDATE proveedor p
SET condicion_pago = 'CREDITO', plazo_credito_dias = sub.plazo
FROM (
    SELECT d.proveedor_id,
           MODE() WITHIN GROUP (ORDER BY (d.fecha_vencimiento - d.fecha_emision)) AS plazo
    FROM documento_cxp d
    WHERE d.fecha_vencimiento IS NOT NULL
      AND d.fecha_vencimiento > d.fecha_emision
      AND d.descripcion ILIKE '%crédito%'
    GROUP BY d.proveedor_id
) sub
WHERE p.id = sub.proveedor_id
  AND p.condicion_pago = 'CONTADO'
  AND p.plazo_credito_dias = 0
  AND sub.plazo > 0;
