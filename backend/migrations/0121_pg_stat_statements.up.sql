-- El contador de consultas de PostgreSQL (11-nov-2026).
--
-- Cuando alguien dice «el sistema está lento», hoy no hay forma de saber QUÉ está lento. Hubo que
-- activar esta extensión a mano en desarrollo para descubrir que UNA consulta del tablero de CxP
-- se llevaba el 92 % del tiempo de esa pantalla: sin ella, el diagnóstico es adivinar.
--
-- NO GUARDA DATOS DE NADIE. PostgreSQL normaliza cada consulta antes de contarla: los valores se
-- reemplazan por marcadores ($1, $2), así que queda la FORMA de la consulta y los tiempos, nunca
-- el monto, el proveedor ni la cédula que llevaba. Es la razón por la que esto se puede mirar
-- desde una pantalla sin exponer información de las empresas.
--
-- NO FALLA SI LA LIBRERÍA NO ESTÁ CARGADA. `pg_stat_statements` necesita cargarse al arrancar el
-- motor (`shared_preload_libraries`), cosa que hace el docker-compose de producción; pero un
-- servidor que actualice el código sin actualizar su compose tendría el motor sin la librería, y
-- ahí un CREATE EXTENSION pelado aborta la migración y el sistema NO LEVANTA. Cambiar un ERP
-- entero por no poder encender una herramienta de diagnóstico es exactamente al revés: acá se
-- avisa y se sigue. La pantalla de consultas lentas ya sabe decir que no está disponible.
DO $$
BEGIN
    CREATE EXTENSION IF NOT EXISTS pg_stat_statements;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_stat_statements no se pudo activar (%). El sistema funciona igual; la pantalla de consultas lentas va a decir que no está disponible. Para encenderla, el motor tiene que arrancar con shared_preload_libraries=pg_stat_statements.', SQLERRM;
END $$;

-- El permiso para mirarlo. `critico` por lo mismo que los errores del sistema: lo que muestra es
-- el SQL del ERP —nombres de tablas, columnas y cómo se cruzan—, que es el mapa de por dónde
-- atacarlo. Los VALORES no están (PostgreSQL ya los reemplazó por $1, $2), pero la estructura sí.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('admin.rendimiento',
     'Administración',
     'Ver las consultas lentas',
     'Permite ver qué consultas se llevan el tiempo del servidor cuando el sistema va lento. Muestra la FORMA de las consultas (PostgreSQL reemplaza los valores por marcadores), pero esa normalización no está garantizada al 100 %: en casos raros puede quedar un valor a la vista.')
ON CONFLICT (codigo) DO NOTHING;

-- Se siembra sobre las parejas (empresa, rol) que YA administran el sistema, igual que la 0107:
-- quien puede tocar roles y permisos es quien sostiene la instalación. No se le estrena la
-- facultad a nadie.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso adm ON adm.id = rp.permiso_id AND adm.codigo = 'admin.roles'
CROSS JOIN permiso p
WHERE p.codigo = 'admin.rendimiento'
ON CONFLICT DO NOTHING;
