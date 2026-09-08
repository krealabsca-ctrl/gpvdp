-- Vuelve `bancos.ver` a ser el permiso único de lectura de Bancos.
--
-- Antes de quitar los permisos por pantalla hay que devolverle `bancos.ver` a todo rol que haya
-- quedado con alguno de ellos: si no, un rol al que le destildaron `bancos.ver` y le dejaron solo
-- «ver saldos» perdería TODO el acceso al módulo al revertir. Bajar una migración no puede dejar a
-- nadie con menos de lo que tenía.
INSERT INTO rol_permiso (rol_id, empresa_id, permiso_id)
SELECT DISTINCT rp.rol_id, rp.empresa_id, base.id
FROM rol_permiso rp
JOIN permiso p ON p.id = rp.permiso_id
CROSS JOIN permiso base
WHERE base.codigo = 'bancos.ver'
  AND p.codigo IN (
    'bancos.ver_clasificar', 'bancos.ver_dashboard', 'bancos.ver_analisis', 'bancos.ver_control',
    'bancos.ver_proyecciones', 'bancos.ver_tc', 'bancos.ver_saldos', 'bancos.ver_conciliacion'
  )
ON CONFLICT DO NOTHING;

-- La descripción original.
UPDATE permiso
SET nombre = 'Ver Bancos',
    descripcion = 'Dashboard, movimientos, cuadre, análisis, proyecciones y catálogo (lectura)'
WHERE codigo = 'bancos.ver';

-- Y se van los ocho. El ON DELETE CASCADE de rol_permiso limpia las concesiones.
DELETE FROM permiso WHERE codigo IN (
  'bancos.ver_clasificar', 'bancos.ver_dashboard', 'bancos.ver_analisis', 'bancos.ver_control',
  'bancos.ver_proyecciones', 'bancos.ver_tc', 'bancos.ver_saldos', 'bancos.ver_conciliacion'
);
