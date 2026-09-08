-- Revierte la consulta por segmento.
--
-- Se borran las dos tablas: el alcance se puede volver a marcar en el catálogo, y los reportes
-- pendientes no son un hecho financiero (no mueven plata ni cambian una partida; la corrección que
-- sí ocurrió quedó en `auditoria_evento` por el camino de reclasificar).
DROP TABLE IF EXISTS movimiento_reporte_segmentacion;
DROP TABLE IF EXISTS rol_clasificacion_consulta;

-- El permiso sale del catálogo; `rol_permiso` lo suelta por su FK en cascada.
DELETE FROM permiso WHERE codigo = 'bancos.ver_mi_segmento';
