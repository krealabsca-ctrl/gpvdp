-- Revierte la mitad de ingreso de «Mi presupuesto».
--
-- Se pierde el alcance configurado (quién ve qué partida de ingreso). No hay dónde guardarlo: es
-- justamente lo que esta migración creó.

DROP TABLE IF EXISTS usuario_clasificacion_consulta;

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'bancos.ver_mi_ingreso');
DELETE FROM permiso WHERE codigo = 'bancos.ver_mi_ingreso';
