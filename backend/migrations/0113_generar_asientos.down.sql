-- Revierte la 0112.

DROP INDEX IF EXISTS ix_asiento_origen_fecha;

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'conta.generar');
DELETE FROM permiso WHERE codigo = 'conta.generar';
