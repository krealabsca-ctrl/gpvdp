-- Facturas «de Contabilidad»: las que no tienen área operativa que las valide.
--
-- El problema real: la validación de área (0027) exige que un validador del departamento asignado
-- confirme la factura antes de la aprobación financiera. Pero hay gasto que por naturaleza es de
-- Contabilidad —honorarios del contador, timbres, comisiones bancarias, Hacienda, auditoría— y no
-- existe un área operativa que pueda dar esa conformidad. Hoy esas facturas se quedan trancadas:
-- el escalamiento (0027) solo procede si el departamento NO tiene validador o la factura está
-- vencida, y de los 13 departamentos de Valle de Paz solo 2 tienen validador asignado.
--
-- Lo que se marca acá NO es «no requiere aprobación». La matriz de firmas por monto se sigue
-- aplicando igual. Lo único que se salta es el control OPERATIVO de un área que no tiene nada que
-- confirmar; la firma financiera, que es el control que protege el pago, queda intacta.
--
-- Tres formas de marcar, decisión del negocio (2026-08-10), porque cubren casos distintos:
--   · proveedor      — «siempre van a ser de conta»: se marca una vez y sus facturas nacen así.
--   · concepto       — todo un rubro (p. ej. «Impuestos»).
--   · clasificación  — el nivel fino de un rubro (p. ej. «Gastos › Comisiones bancarias»).
--   · por factura    — el caso a caso, con motivo obligatorio.
--
-- El de la factura es un OVERRIDE de TRES estados (NULL / true / false) y no un simple booleano:
-- sin él, marcar un concepto forzaría a TODA factura de ese rubro a saltarse la validación,
-- incluida una que sí sea de Logística. Con él, Contabilidad puede decir «esta sí» y también
-- «esta NO, que la valide el área» sin tener que desmarcar el catálogo entero.

BEGIN;

-- 1) La marca a nivel de proveedor: es la que captura el «siempre».
ALTER TABLE proveedor
    ADD COLUMN IF NOT EXISTS es_contabilidad boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN proveedor.es_contabilidad IS
    'Las facturas de este proveedor son de Contabilidad: no requieren validación de área.';

-- 2) La marca por rubro, en los dos niveles del catálogo.
ALTER TABLE concepto
    ADD COLUMN IF NOT EXISTS es_contabilidad boolean NOT NULL DEFAULT false;
ALTER TABLE clasificacion
    ADD COLUMN IF NOT EXISTS es_contabilidad boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN concepto.es_contabilidad IS
    'Todo el rubro es de Contabilidad: sus facturas no requieren validación de área.';
COMMENT ON COLUMN clasificacion.es_contabilidad IS
    'Esta clasificación es de Contabilidad: sus facturas no requieren validación de área.';

-- 3) El override por factura (NULL = hereda de proveedor/catálogo) con su rastro.
ALTER TABLE documento_cxp
    ADD COLUMN IF NOT EXISTS es_contabilidad boolean,
    ADD COLUMN IF NOT EXISTS contabilidad_motivo text,
    ADD COLUMN IF NOT EXISTS contabilidad_marcado_por uuid REFERENCES usuario (id),
    ADD COLUMN IF NOT EXISTS contabilidad_marcado_en timestamptz;

COMMENT ON COLUMN documento_cxp.es_contabilidad IS
    'Override de la marca: NULL hereda de proveedor/concepto/clasificación, true fuerza «de Contabilidad», false fuerza que la valide el área.';

-- Índice parcial: la bandeja filtra «solo las de Contabilidad» y son la minoría.
CREATE INDEX IF NOT EXISTS idx_docxp_contabilidad
    ON documento_cxp (empresa_id, estado)
    WHERE es_contabilidad IS TRUE;

-- 4) Los permisos nuevos. Son propios y asignables (no se amarran a un rol) para que el negocio
--    pueda moverlos desde Configuración › Seguridad sin tocar código.
--
--    Son DOS y separados a propósito: marcar es un acto de segmentación (lo hace Contabilidad al
--    clasificar la factura) y aprobar es la firma. Con un solo permiso, quien puede decir «esta no
--    necesita validación de área» sería automáticamente quien la aprueba, y eso convierte la marca
--    en una autofirma.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico)
VALUES
    ('cxp.aprobar_contabilidad', 'Cuentas por pagar', 'Aprobar facturas de Contabilidad',
     'Aprobar las facturas marcadas como de Contabilidad sin que pasen por la validación de área (la matriz de firmas por monto se sigue aplicando)',
     true),
    ('cxp.marcar_contabilidad', 'Cuentas por pagar', 'Marcar facturas como de Contabilidad',
     'Marcar (o desmarcar) una factura, un proveedor o un rubro como «de Contabilidad»: sus facturas no requieren validación de área',
     false)
ON CONFLICT (codigo) DO NOTHING;

-- Se conceden a los roles que el negocio definió, en TODAS las empresas existentes. La concesión
-- para las empresas nuevas vive en rbac.MatrizDefault (si no está ahí, una empresa nueva nace sin
-- el permiso y `aplicar-faltantes` nunca se lo da).
--
-- El CROSS JOIN con empresa es obligatorio y no un adorno: los roles base son GLOBALES
-- (rol.empresa_id IS NULL) y la empresa la lleva `rol_permiso`. Sacar el empresa_id de `rol`
-- inserta NULL y la migración muere contra el NOT NULL.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT e.id, r.id, p.id
FROM empresa e
CROSS JOIN rol r
JOIN permiso p ON p.codigo = 'cxp.aprobar_contabilidad'
WHERE r.codigo IN ('DIRECTOR_FINANCIERO', 'SUPERVISOR_FINANCIERO', 'GERENCIA_GENERAL')
  AND (r.empresa_id IS NULL OR r.empresa_id = e.id)
ON CONFLICT DO NOTHING;

-- Marcar lo hace Contabilidad: el auxiliar que segmenta la factura y su supervisor.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT e.id, r.id, p.id
FROM empresa e
CROSS JOIN rol r
JOIN permiso p ON p.codigo = 'cxp.marcar_contabilidad'
WHERE r.codigo IN ('DIRECTOR_FINANCIERO', 'SUPERVISOR_FINANCIERO', 'AUXILIAR_FINANCIERO')
  AND (r.empresa_id IS NULL OR r.empresa_id = e.id)
ON CONFLICT DO NOTHING;

COMMIT;
