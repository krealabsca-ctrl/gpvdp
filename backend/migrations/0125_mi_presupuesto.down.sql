-- Revierte «Mi presupuesto». No hay datos que devolver: la migración solo creó el permiso y el
-- alcance vive en `departamento_validador`, que es anterior y la usa CxP.

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'bancos.ver_mi_presupuesto');
DELETE FROM permiso WHERE codigo = 'bancos.ver_mi_presupuesto';
