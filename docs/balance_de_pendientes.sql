-- Balance de pendientes de datos — GPVDP
--
-- Para correr EN PRODUCCIÓN. Es solo de lectura: ningún INSERT, UPDATE ni DELETE.
-- Contesta «cuánto dato falta cargar», que es distinto de «qué falta construir»: el código es el
-- mismo en los dos lados, los datos no (los IBANes están en producción y no en local).
--
-- Cómo correrla en el servidor:
--     docker compose exec -T db psql -U gpvdp -d gpvdp -f balance_de_pendientes.sql
-- o pegando el contenido dentro de psql.
--
-- Cambiá el id de abajo si se mide otra empresa (Coopeprofa, Memorial Pets).

\set empresa '427f8851-1e94-4c13-abbd-3f7bb9f456e9'

WITH e AS (SELECT :'empresa'::uuid AS id)

-- 1. Lo que hace falta para PAGAR. Sin IBAN el banco rechaza la línea; sin correo no sale el
--    comprobante al proveedor.
SELECT 1 AS orden, 'Proveedores con IBAN válido' AS dato,
       count(*) FILTER (WHERE iban ~ '^[Cc][Rr][0-9]{20}$')::text || ' de ' || count(*)::text AS valor
FROM proveedor, e WHERE empresa_id = e.id
UNION ALL
SELECT 2, 'Proveedores con correo',
       count(*) FILTER (WHERE COALESCE(email, '') <> '')::text || ' de ' || count(*)::text
FROM proveedor, e WHERE empresa_id = e.id
UNION ALL
SELECT 3, 'Cuentas bancarias sin IBAN',
       count(*) FILTER (WHERE COALESCE(iban, '') = '')::text || ' de ' || count(*)::text
FROM cuenta_bancaria, e WHERE empresa_id = e.id

-- 2. Nomenclatura del pago (A8, A9…). Un código en un concepto se hereda a todas sus
--    clasificaciones, así que pocos códigos cubren muchos rubros.
UNION ALL
SELECT 4, 'Clasificaciones con código de pago',
       count(*) FILTER (WHERE COALESCE(codigo_pago, '') <> '')::text || ' de ' || count(*)::text
FROM clasificacion, e WHERE empresa_id = e.id
UNION ALL
SELECT 5, 'Conceptos con código de pago',
       count(*) FILTER (WHERE COALESCE(codigo_pago, '') <> '')::text || ' de ' || count(*)::text
FROM concepto, e WHERE empresa_id = e.id

-- 3. Clasificación pendiente: una factura sin gasto no se puede atribuir a un departamento ni
--    entra en el presupuesto.
UNION ALL
SELECT 6, 'Facturas sin clasificar',
       count(*) FILTER (WHERE clasificacion_id IS NULL)::text || ' de ' || count(*)::text
FROM documento_cxp, e WHERE empresa_id = e.id
UNION ALL
SELECT 7, 'Movimientos «No identificado»',
       count(*) FILTER (WHERE estado_clasificacion = 'NO_IDENTIFICADO')::text || ' de ' || count(*)::text
FROM movimiento_bancario, e WHERE empresa_id = e.id AND incluido

-- 4. El puente Bancos↔CxP. Cero enlaces con facturas en PAGADO significa que la plata salió y
--    nadie la cruzó contra su factura.
UNION ALL
SELECT 8, 'Facturas en PAGADO (esperan conciliación)',
       count(*)::text || ' · ₡' || to_char(COALESCE(sum(total_crc), 0), 'FM999,999,999.00')
FROM documento_cxp, e WHERE empresa_id = e.id AND estado = 'PAGADO'
UNION ALL
SELECT 9, 'Pagos CxP enlazados al banco',
       count(*)::text
FROM movimiento_bancario, e
WHERE empresa_id = e.id AND (documento_cxp_id IS NOT NULL OR remesa_id IS NOT NULL)

-- 5. Limpieza: un lote sin facturas baja una macro vacía, que se sube al banco igual que una buena
--    y no paga a nadie.
UNION ALL
SELECT 10, 'Lotes de pago vacíos',
       count(*)::text
FROM lote_pago lp, e
WHERE lp.empresa_id = e.id AND NOT EXISTS (SELECT 1 FROM documento_cxp d WHERE d.lote_id = lp.id)
UNION ALL
SELECT 11, 'Remesas sin facturas',
       count(*)::text
FROM remesa_pago r, e
WHERE r.empresa_id = e.id AND NOT EXISTS (SELECT 1 FROM documento_cxp d WHERE d.remesa_id = r.id)
UNION ALL
SELECT 12, 'Traslados sin pareja',
       count(*)::text
FROM movimiento_bancario, e WHERE empresa_id = e.id AND es_traslado AND incluido

ORDER BY orden;

-- La cartera por estado: dónde está trancada y cuánta plata representa cada tramo.
SELECT estado,
       count(*) AS facturas,
       '₡' || to_char(sum(total_crc), 'FM999,999,999.00') AS monto
FROM documento_cxp
WHERE empresa_id = :'empresa'::uuid
GROUP BY estado
ORDER BY count(*) DESC;
