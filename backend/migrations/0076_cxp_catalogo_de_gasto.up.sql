-- Contabilidad puede abrir sus propios rubros de gasto.
--
-- ── EL PROBLEMA MEDIDO (2026-09-03, Valle de Paz) ──────────────────────────
--
-- Una factura de un gasto que no está en el catálogo no se puede clasificar, y sin clasificar no
-- hay departamento que deducir: queda esperando una validación de área que nunca llega. Había
-- **834 facturas sin clasificar** de las 938 que esperaban validación, y solo **4 de 22 conceptos**
-- estaban marcados visibles para CxP.
--
-- Escribir el catálogo era exclusivo de `bancos.catalogo`, que abre además bancos, cuentas, la
-- naturaleza (el EBITDA) y la visibilidad. Darle eso a Contabilidad para que pudiera abrir un rubro
-- era demasiado; no darle nada la dejaba trancada.
--
-- ── EL ALCANCE ES LO QUE HACE DECENTE EL PERMISO ───────────────────────────
--
-- `cxp.catalogo` solo alcanza para los rubros marcados visibles para CxP, y lo que crea nace
-- visible para CxP. Crear y renombrar nada más: apagar, fusionar y declarar la naturaleza siguen
-- siendo de Bancos, porque el catálogo es COMPARTIDO y esas tres tocan la clasificación bancaria
-- histórica y el resultado de la empresa.

INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico) VALUES
  ('cxp.catalogo', 'Cuentas por pagar', 'Abrir rubros de gasto',
   'Crear y renombrar conceptos y clasificaciones de gasto visibles para CxP (no apaga, no fusiona, no declara naturaleza)',
   false)
ON CONFLICT (codigo) DO NOTHING;

-- Se lo damos a quien YA clasifica facturas: es quien se topa con el gasto que no existe.
--
-- Se resuelve por el permiso `cxp.clasificar` y no por una lista de roles a mano, para que también
-- lo reciban los roles CUSTOM que hoy clasifican —no están en `rbac.MatrizDefault`, así que
-- `aplicar-faltantes` no los recorre y se quedarían sin él—.
INSERT INTO rol_permiso (rol_id, empresa_id, permiso_id)
SELECT rp.rol_id, rp.empresa_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'cxp.clasificar'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'cxp.catalogo'
ON CONFLICT DO NOTHING;
