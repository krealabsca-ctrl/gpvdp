-- La lectura de Bancos se partió en un permiso POR PANTALLA.
--
-- Antes `bancos.ver` abría de una sola vez las diez pantallas de lectura del módulo, así que era
-- imposible darle a la tesorera la captura de saldos sin mostrarle también el EBITDA, el
-- presupuesto y las proyecciones de la empresa. Ahora `bancos.ver` es solo la ENTRADA (los
-- catálogos de referencia) y cada pantalla con cifras pide su propio permiso.
--
-- ── QUÉ HACE ESTA MIGRACIÓN Y POR QUÉ ──────────────────────────────────────
--
-- Le da los ocho permisos nuevos a TODO rol que hoy ya tenga `bancos.ver`, en cada empresa donde
-- lo tenga. Sin esto, el cambio sería una quita de acceso silenciosa: gente que hoy abre el
-- dashboard mañana vería «Sin acceso» sin que nadie hubiera decidido eso.
--
-- Es imprescindible para los roles **CUSTOM**: no están en `rbac.MatrizDefault`, así que
-- `aplicar-faltantes` no los recorre y se quedarían congelados. Es el mismo hueco que ya nos costó
-- una vez con SUPERVISOR_PISO.
--
-- Lo que NO hace: no le quita nada a nadie. Restringir es ahora una decisión del negocio, que se
-- toma destildando pantallas en Configuración › Seguridad.

-- 1. Los permisos nuevos en el catálogo. `critico = false`: son de lectura.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico) VALUES
  ('bancos.ver_clasificar',   'Bancos', 'Ver la bandeja de clasificación', 'Movimientos, reglas, patrones y traslados propuestos (lectura)', false),
  ('bancos.ver_dashboard',    'Bancos', 'Ver el dashboard y el cuadre',    'KPIs del período, cuadre, serie mensual, calendario y resumen por cuenta', false),
  ('bancos.ver_analisis',     'Bancos', 'Ver análisis y tendencias',       'Cada partida contra su propia historia', false),
  ('bancos.ver_control',      'Bancos', 'Ver control y presupuesto',       'Gasto por departamento y sede contra el presupuesto', false),
  ('bancos.ver_proyecciones', 'Bancos', 'Ver proyecciones',                'Escenarios de cierre de mes', false),
  ('bancos.ver_tc',           'Bancos', 'Ver tipo de cambio',              'Estado del TC del mes y última sincronización', false),
  ('bancos.ver_saldos',       'Bancos', 'Ver saldos diarios',              'Saldo por cuenta y día y el checklist de carga (tesorería)', false),
  ('bancos.ver_conciliacion', 'Bancos', 'Ver actas de conciliación',       'El acta mensual de cada cuenta y sus partidas en tránsito', false)
ON CONFLICT (codigo) DO NOTHING;

-- 2. La descripción de `bancos.ver` ya no es la que era: dejó de abrir todo el módulo.
UPDATE permiso
SET nombre = 'Entrar a Bancos (catálogos)',
    descripcion = 'Leer cuentas, conceptos, clasificaciones y el estado del período. NO alcanza para ver movimientos ni cifras: cada pantalla pide su propio permiso'
WHERE codigo = 'bancos.ver';

-- 3. Todo rol con `bancos.ver` conserva lo que hoy ve.
--
-- El producto cartesiano es a propósito: se cruza cada (rol, empresa) que YA tiene `bancos.ver`
-- con los ocho permisos nuevos. Así el alcance por empresa se respeta —un rol que solo lee Bancos
-- en una empresa no gana acceso en otra—.
INSERT INTO rol_permiso (rol_id, empresa_id, permiso_id)
SELECT rp.rol_id, rp.empresa_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'bancos.ver'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo IN (
  'bancos.ver_clasificar', 'bancos.ver_dashboard', 'bancos.ver_analisis', 'bancos.ver_control',
  'bancos.ver_proyecciones', 'bancos.ver_tc', 'bancos.ver_saldos', 'bancos.ver_conciliacion'
)
ON CONFLICT DO NOTHING;
