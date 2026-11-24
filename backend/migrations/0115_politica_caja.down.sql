DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'bancos.politica_caja');
DELETE FROM permiso WHERE codigo = 'bancos.politica_caja';

DROP INDEX IF EXISTS ix_politica_caja_empresa;
DROP TABLE IF EXISTS politica_caja;
