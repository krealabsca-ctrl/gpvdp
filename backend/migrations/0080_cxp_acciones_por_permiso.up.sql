-- Las acciones en lote de CxP se autorizan por PERMISO, no por código de rol.
--
-- ── EL DEFECTO ──────────────────────────────────────────────────────────────
--
-- La ruta masiva `/cxp/documentos/transicion-masiva` está detrás de un solo permiso
-- (`cxp.revisar`) pero transporta diez acciones: revisar, aprobar, programar, pagar, conciliar,
-- denegar, anular, liquidar, rebotar y reintentar. Como ese permiso es demasiado grueso, el
-- servicio hacía una segunda verificación… contra una lista de CÓDIGOS DE ROL escrita a mano.
--
-- El sistema permite crear roles a medida y marcarles permisos desde la matriz —es una decisión
-- del negocio— pero esa lista solo conocía los seis roles base. Un rol nuevo con TODO CxP marcado
-- recibía «el rol no puede ejecutar esta acción»: la matriz decía sí y el código decía no, y la
-- única salida era editar Go. Pasó en producción el 9 de setiembre de 2026: el rol «Supervisor
-- Contable» no podía liquidar unos viáticos.
--
-- Ahora cada acción pide el permiso de su ruta individual. Y dos acciones que no tenían permiso
-- propio lo estrenan acá, porque su recorte vivía únicamente en la lista de roles.
--
-- ── LA REGLA DE ESTA MIGRACIÓN: NADIE GANA NI PIERDE ────────────────────────
--
-- Los permisos nuevos se le dan exactamente a los roles que HOY pueden hacer esa acción, en cada
-- empresa donde puedan. Así el despliegue no cambia lo que nadie puede hacer; lo que cambia es que
-- de ahora en adelante se puede marcar a un rol a medida sin tocar código.
--
-- ADMIN no aparece: tiene bypass en el checker.

-- 1. Los permisos nuevos. `critico = true` en los dos: uno deja de deber una factura y el otro
--    declara lo que hizo el banco con un pago ya enviado.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico) VALUES
  ('cxp.anular', 'Cuentas por pagar', 'Denegar y anular facturas',
   'Sacar la factura del flujo de pago, con motivo', true),
  ('cxp.resultado_pago', 'Cuentas por pagar', 'Registrar el resultado del banco',
   'Marcar un pago como rebotado o reintentarlo', true)
ON CONFLICT (codigo) DO NOTHING;

-- 2. `cxp.revisar` ya cubre liquidar viáticos: se aclara en el catálogo para que nadie busque un
--    permiso de «liquidar» que no existe.
UPDATE permiso
SET descripcion = 'Marcar como revisadas. Incluye liquidar viáticos (sin pago)'
WHERE codigo = 'cxp.revisar';

-- 3. `cxp.anular` a quien ya podía denegar/anular: Supervisor y Director Financiero.
--
-- Se cruza con `cxp.revisar` a propósito: sin ese permiso el rol no llega a la ruta masiva, así
-- que darle `cxp.anular` sería marcar un acceso que no existe.
INSERT INTO rol_permiso (rol_id, empresa_id, permiso_id)
SELECT rp.rol_id, rp.empresa_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'cxp.revisar'
JOIN rol ro ON ro.id = rp.rol_id
   AND ro.codigo IN ('SUPERVISOR_FINANCIERO', 'DIRECTOR_FINANCIERO')
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'cxp.anular'
ON CONFLICT DO NOTHING;

-- 4. `cxp.resultado_pago` a quien ya podía rebotar/reintentar: solo Dirección Financiera.
INSERT INTO rol_permiso (rol_id, empresa_id, permiso_id)
SELECT rp.rol_id, rp.empresa_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'cxp.tesoreria'
JOIN rol ro ON ro.id = rp.rol_id AND ro.codigo = 'DIRECTOR_FINANCIERO'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'cxp.resultado_pago'
ON CONFLICT DO NOTHING;
