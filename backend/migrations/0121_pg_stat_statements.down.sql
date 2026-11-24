-- Apagar el contador de consultas.
--
-- Se pierden los tiempos acumulados: la extensión los guarda en memoria compartida, no en una
-- tabla, así que no hay nada que respaldar ni que recuperar. Volver a crearla empieza de cero.
--
-- Tolerante igual que la subida: si la extensión no estaba (porque el motor nunca cargó la
-- librería), quitarla no debe tumbar la bajada.
DELETE FROM rol_permiso
 WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'admin.rendimiento');
DELETE FROM permiso WHERE codigo = 'admin.rendimiento';

DO $$
BEGIN
    DROP EXTENSION IF EXISTS pg_stat_statements;
EXCEPTION WHEN OTHERS THEN
    RAISE NOTICE 'pg_stat_statements no se pudo quitar (%).', SQLERRM;
END $$;
