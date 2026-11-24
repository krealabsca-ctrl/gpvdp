-- 0115 · Los ratios del ciclo de efectivo: DSO, DPO y el ciclo de conversión.
--
-- ── POR QUÉ UN PERMISO PROPIO ───────────────────────────────────────────────────────────────
--
-- Porque cruzan módulos y tienen otra audiencia. El DSO sale de la cartera por cobrar y el DPO
-- de la cartera por pagar: quien los mira es dirección, no quien opera CxC o CxP. Colgarlos de
-- `cxc.ver` o `cxp.ver` obligaría a dar acceso al detalle de un módulo entero para ver tres
-- números agregados, y al revés —reusar `bancos.ver_analisis`— dejaría ver la cartera de CxC a
-- quien solo tiene Bancos.
--
-- Son de LECTURA y nada más: este módulo no escribe una sola fila.

INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('ratios.ver',
     'Análisis financiero',
     'Ver los ratios del ciclo de efectivo',
     'DSO (días para cobrar), DPO (días para pagar) y el ciclo de conversión de efectivo. Son agregados: no muestran el detalle de ninguna factura ni de ningún contrato.')
ON CONFLICT (codigo) DO NOTHING;

-- Lo reciben los roles que ya ven el análisis financiero de Bancos: es el mismo trabajo —mirar
-- la salud del negocio— visto desde la cartera en vez de desde el banco.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso base ON base.id = rp.permiso_id AND base.codigo = 'bancos.ver_analisis'
CROSS JOIN permiso p
WHERE p.codigo = 'ratios.ver'
ON CONFLICT DO NOTHING;

-- ── Los índices que el cálculo necesita ─────────────────────────────────────────────────────
--
-- El countback recorre los meses hacia atrás agotando la cartera, así que agrupa por mes de
-- emisión sobre toda la ventana. Sin estos índices son dos seq scans sobre las tablas más
-- grandes del sistema cada vez que alguien abre la pantalla.
CREATE INDEX IF NOT EXISTS ix_docxp_emision_empresa
    ON documento_cxp (empresa_id, fecha_emision);

CREATE INDEX IF NOT EXISTS ix_cargo_cxc_vence_empresa
    ON cargo_cxc (empresa_id, vence_en);
