-- Revertir la recepción de facturas por correo.
--
-- Se quitan los grants antes que los permisos (FK) y las tablas en orden inverso al de creación.
-- El usuario técnico se deja: puede haber quedado como `creado_por` de documentos ya creados y
-- `documento_cxp.creado_por` es FK a usuario, así que borrarlo fallaría o dejaría el rastro roto.

DELETE FROM rol_permiso WHERE permiso_id IN
  (SELECT id FROM permiso WHERE codigo IN ('cxp.recepcion', 'cxp.fuentes'));
DELETE FROM permiso WHERE codigo IN ('cxp.recepcion', 'cxp.fuentes');

DROP TABLE IF EXISTS cxp_recepcion;
DROP TABLE IF EXISTS cxp_fuente_recepcion;
DROP TABLE IF EXISTS empresa_cedula;
