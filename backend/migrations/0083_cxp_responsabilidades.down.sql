-- Revierte 0082. El orden importa: los períodos apuntan a las responsabilidades.
DELETE FROM rol_permiso WHERE permiso_id IN (
  SELECT id FROM permiso WHERE codigo IN (
    'cxp.responsabilidades.ver',
    'cxp.responsabilidades.ver_mias',
    'cxp.responsabilidades.declarar',
    'cxp.responsabilidades.abrir_mes',
    'cxp.responsabilidades.cerrar'
  )
);
DELETE FROM permiso WHERE codigo IN (
  'cxp.responsabilidades.ver',
  'cxp.responsabilidades.ver_mias',
  'cxp.responsabilidades.declarar',
  'cxp.responsabilidades.abrir_mes',
  'cxp.responsabilidades.cerrar'
);

DROP TABLE IF EXISTS responsabilidad_periodo;
DROP TABLE IF EXISTS responsabilidad_responsable;
DROP TABLE IF EXISTS responsabilidad_cxp;
