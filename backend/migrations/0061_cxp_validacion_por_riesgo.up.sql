-- Validación de área POR RIESGO, no por organigrama.
--
-- La regla era «todo se valida, salvo lo que marques». Con los datos reales de Valle de Paz eso
-- significa pedir confirmación humana para 4.533 facturas, y el 81,8 % de ellas son de ₡100.000 o
-- menos: apenas el 7,8 % del dinero. Al revés, el 3,5 % que supera ₡1.000.000 concentra el 74,1 %.
-- Además 159 de los 648 proveedores (los recurrentes) explican el 94 % del monto: no son riesgo,
-- son rutina.
--
-- Decisión del negocio (2026-08-13): se invierte. Nada requiere validación de área salvo que
-- dispare un criterio de RIESGO. Es lo que hacen los ERP grandes con el three-way match — lo que
-- calza contra un compromiso previo se paga sin intervención, y solo la excepción llega a una
-- persona. Acá no hay órdenes de compra, así que el compromiso se aproxima con el historial del
-- propio proveedor.
--
-- Criterios (parametrizables por empresa):
--   MONTO      — sobre el umbral, va a validación. Umbral inicial ₡250.000.
--   PROVEEDOR  — con 2 facturas históricas o menos: proveedor nuevo o esporádico. Son 364
--                proveedores pero solo ₡25,8 M: el riesgo está ahí, no la plata.
--   DESVÍO     — la factura se aparta más de X % del promedio histórico de ese proveedor. Es lo
--                que detecta el recibo del agua que siempre fue ₡70.000 y viene por ₡700.000.
--
-- Medido sobre los datos reales: MONTO > ₡250.000 O proveedor esporádico deja 833 facturas en
-- validación (18,4 % del volumen) cubriendo el 86,3 % del dinero.
--
-- Lo que NO cambia: la matriz de firmas por monto y la segregación de funciones. Eso es aprobación
-- financiera, no validación de área.

BEGIN;

-- 1) Parámetros por empresa (mismo patrón clave/valor que cxc_parametro).
CREATE TABLE IF NOT EXISTS cxp_parametro (
    empresa_id      uuid NOT NULL REFERENCES empresa (id) ON DELETE CASCADE,
    clave           text NOT NULL,
    valor           text NOT NULL,
    descripcion     text NOT NULL DEFAULT '',
    actualizado_en  timestamptz NOT NULL DEFAULT now(),
    actualizado_por uuid REFERENCES usuario (id),
    PRIMARY KEY (empresa_id, clave)
);

INSERT INTO cxp_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, v.clave, v.valor, v.descripcion
FROM empresa e
CROSS JOIN (VALUES
    ('VALIDACION_UMBRAL_MONTO', '250000',
     'Desde este monto (CRC) la factura requiere que el área confirme la conformidad'),
    ('VALIDACION_PROVEEDOR_NUEVO_MAX', '2',
     'Un proveedor con esta cantidad de facturas históricas o menos se considera nuevo/esporádico: sus facturas se validan'),
    ('VALIDACION_DESVIO_PCT', '50',
     'Si la factura se aparta más de este porcentaje del promedio histórico del proveedor, se valida (0 = desactivado)')
) AS v(clave, valor, descripcion)
ON CONFLICT (empresa_id, clave) DO NOTHING;

-- 2) El veredicto se GUARDA en la factura, no se recalcula al leer.
--
-- Es un hecho del momento en que se revisó: si mañana se sube el umbral, una factura que ya pasó
-- por validación no puede dejar de decir que la necesitaba. Misma lección que el sello de las
-- facturas «de Contabilidad»: el pasado no se reescribe.
ALTER TABLE documento_cxp
    ADD COLUMN IF NOT EXISTS requiere_validacion boolean,
    ADD COLUMN IF NOT EXISTS validacion_motivo text;

COMMENT ON COLUMN documento_cxp.requiere_validacion IS
    'Si el área tiene que confirmar la conformidad. NULL = todavía no evaluado (se evalúa al revisar).';
COMMENT ON COLUMN documento_cxp.validacion_motivo IS
    'Por qué requiere validación: MONTO, PROVEEDOR_NUEVO o DESVIO. Vacío cuando no la requiere.';

CREATE INDEX IF NOT EXISTS idx_docxp_requiere_validacion
    ON documento_cxp (empresa_id, estado)
    WHERE requiere_validacion IS TRUE;

-- 3) Evaluar lo que ya existe y todavía está abierto. Sin esto, las 4.467 facturas abiertas se
--    quedarían con NULL y la cola de validación aparecería vacía —o completa— por accidente.
WITH params AS (
    SELECT empresa_id,
           MAX(valor) FILTER (WHERE clave = 'VALIDACION_UMBRAL_MONTO')::numeric        AS umbral,
           MAX(valor) FILTER (WHERE clave = 'VALIDACION_PROVEEDOR_NUEVO_MAX')::int     AS max_nuevo,
           MAX(valor) FILTER (WHERE clave = 'VALIDACION_DESVIO_PCT')::numeric          AS desvio_pct
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
               WHEN p.desvio_pct > 0 AND h.promedio > 0
                    AND ABS(d.total_crc - h.promedio) > h.promedio * p.desvio_pct / 100 THEN 'DESVIO'
               ELSE ''
           END AS motivo
    FROM documento_cxp d
    JOIN params p ON p.empresa_id = d.empresa_id
    LEFT JOIN hist h ON h.empresa_id = d.empresa_id AND h.proveedor_id = d.proveedor_id
    WHERE d.tipo = 'CXP' AND d.estado IN ('RECIBIDO', 'REVISADO', 'VALIDADO_DEPTO')
)
UPDATE documento_cxp d
SET requiere_validacion = (e.motivo <> ''),
    validacion_motivo   = NULLIF(e.motivo, '')
FROM evaluado e
WHERE d.id = e.id AND d.requiere_validacion IS NULL;

-- Las que ya fueron validadas por el área conservan ese hecho: requerían validación y la tuvieron.
UPDATE documento_cxp
SET requiere_validacion = true,
    validacion_motivo   = COALESCE(validacion_motivo, 'MONTO')
WHERE validado_depto_en IS NOT NULL AND requiere_validacion IS NOT TRUE;

-- 4) Permiso para mover los umbrales: cambiarlos mueve cuánto gasto pasa sin revisión humana.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico)
VALUES ('cxp.parametros', 'Cuentas por pagar', 'Configurar los umbrales de validación',
        'Define desde qué monto y en qué casos una factura requiere que el área confirme la conformidad (cambia cuánto gasto se paga sin revisión humana)',
        true)
ON CONFLICT (codigo) DO NOTHING;

INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT e.id, r.id, p.id
FROM empresa e
CROSS JOIN rol r
JOIN permiso p ON p.codigo = 'cxp.parametros'
WHERE r.codigo IN ('DIRECTOR_FINANCIERO', 'GERENCIA_GENERAL')
  AND (r.empresa_id IS NULL OR r.empresa_id = e.id)
ON CONFLICT DO NOTHING;

COMMIT;
