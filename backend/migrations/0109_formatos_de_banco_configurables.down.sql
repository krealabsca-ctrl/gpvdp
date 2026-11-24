-- Revierte la 0108.
--
-- Se pierden los formatos configurados. El importador vuelve a reconocer SOLO los seis que vienen
-- en el código, que es como venía funcionando: los archivos de un banco agregado desde la
-- pantalla dejarían de reconocerse hasta que alguien los vuelva a configurar.
--
-- Lo ya importado no se toca: `importacion.banco` guarda el nombre como texto, sin llave foránea
-- a esta tabla, justamente para que el historial no dependa de una configuración que puede
-- cambiar o borrarse.
DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'bancos.formatos');
DELETE FROM permiso WHERE codigo = 'bancos.formatos';

DROP TABLE IF EXISTS formato_banco;
