-- Calibración del criterio de DESVÍO: solo aplica sobre facturas que valgan la pena mirar.
--
-- La 0061 evaluaba el desvío sobre cualquier monto y el resultado real lo desmintió: de las 4.464
-- facturas abiertas de Valle de Paz, el desvío solo arrastraba 1.185 — el total a validar subía a
-- 2.008 (45 %), muy lejos del 18-20 % que la regla busca. La causa es que el promedio de un
-- proveedor con facturación variable se desvía todo el tiempo, y escalar a una persona un recibo de
-- ₡3.000 porque el promedio era ₡1.000 es ruido puro: el desvío importa cuando la plata importa.
--
-- Con piso, medido sobre los mismos datos:
--
--   piso        facturas que agrega el desvío
--   ₡0                1.185
--   ₡25.000             413
--   ₡50.000             284
--   ₡100.000            145      ← elegido
--   ₡150.000             87
--
-- Resultado total con piso ₡100.000: 969 facturas a validar (21,7 % del volumen) cubriendo el
-- 88,9 % del monto. Cubre MÁS dinero que la regla de solo monto+proveedor (86,3 %) agregando
-- apenas 145 revisiones, que son justamente las anomalías.

BEGIN;

INSERT INTO cxp_parametro (empresa_id, clave, valor, descripcion)
SELECT id, 'VALIDACION_DESVIO_PISO_MONTO', '100000',
       'El criterio de desvío solo se evalúa sobre facturas que superen este monto (CRC): abajo, una anomalía porcentual no representa riesgo'
FROM empresa
ON CONFLICT (empresa_id, clave) DO NOTHING;

-- Recalcular lo abierto con el criterio calibrado. Solo lo que TODAVÍA no pasó por validación: una
-- factura ya validada conserva el hecho de que la necesitaba.
WITH params AS (
    SELECT empresa_id,
           MAX(valor) FILTER (WHERE clave = 'VALIDACION_UMBRAL_MONTO')::numeric        AS umbral,
           MAX(valor) FILTER (WHERE clave = 'VALIDACION_PROVEEDOR_NUEVO_MAX')::int     AS max_nuevo,
           MAX(valor) FILTER (WHERE clave = 'VALIDACION_DESVIO_PCT')::numeric          AS desvio_pct,
           MAX(valor) FILTER (WHERE clave = 'VALIDACION_DESVIO_PISO_MONTO')::numeric   AS desvio_piso
    FROM cxp_parametro GROUP BY empresa_id
),
hist AS (
    SELECT empresa_id, proveedor_id, COUNT(*) AS facturas, AVG(total_crc) AS promedio
    FROM documento_cxp WHERE tipo = 'CXP' GROUP BY 1, 2
),
evaluado AS (
    SELECT d.id,
           CASE
               WHEN d.total_crc > p.umbral THEN 'MONTO'
               WHEN COALESCE(h.facturas, 0) <= p.max_nuevo THEN 'PROVEEDOR_NUEVO'
               WHEN p.desvio_pct > 0 AND h.promedio > 0 AND d.total_crc > p.desvio_piso
                    AND ABS(d.total_crc - h.promedio) > h.promedio * p.desvio_pct / 100 THEN 'DESVIO'
               ELSE ''
           END AS motivo
    FROM documento_cxp d
    JOIN params p ON p.empresa_id = d.empresa_id
    LEFT JOIN hist h ON h.empresa_id = d.empresa_id AND h.proveedor_id = d.proveedor_id
    WHERE d.tipo = 'CXP' AND d.estado IN ('RECIBIDO', 'REVISADO', 'VALIDADO_DEPTO')
      AND d.validado_depto_en IS NULL
)
UPDATE documento_cxp d
SET requiere_validacion = (e.motivo <> ''),
    validacion_motivo   = NULLIF(e.motivo, '')
FROM evaluado e
WHERE d.id = e.id;

COMMIT;
