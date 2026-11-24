-- Revierte la 0107.
--
-- Se pierde el historial de errores registrados. Nada se apoya en él —ninguna otra tabla lo
-- referencia y ningún cálculo lo lee—, así que la reversa es limpia: el sistema vuelve a dejar la
-- referencia solo en el log, que es como venía funcionando.
--
-- El permiso se va primero de `rol_permiso` por la llave foránea; borrarlo al revés dejaría la
-- fila huérfana y el DELETE del permiso fallaría.
DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'admin.incidentes');
DELETE FROM permiso WHERE codigo = 'admin.incidentes';

DROP TABLE IF EXISTS incidente;
