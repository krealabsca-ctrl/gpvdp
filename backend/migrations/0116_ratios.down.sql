DROP INDEX IF EXISTS ix_cargo_cxc_vence_empresa;
DROP INDEX IF EXISTS ix_docxp_emision_empresa;

DELETE FROM rol_permiso WHERE permiso_id IN (SELECT id FROM permiso WHERE codigo = 'ratios.ver');
DELETE FROM permiso WHERE codigo = 'ratios.ver';
