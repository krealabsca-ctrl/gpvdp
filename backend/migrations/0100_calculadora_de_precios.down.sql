-- Revierte la 0099. El orden importa: las que tienen llave foránea primero.
DELETE FROM rol_permiso WHERE permiso_id IN (
    SELECT id FROM permiso WHERE codigo IN ('precios.ver','precios.editar')
);
DELETE FROM permiso WHERE codigo IN ('precios.ver','precios.editar');

DROP TABLE IF EXISTS receta_linea;
DROP TABLE IF EXISTS plan_precio;
DROP TABLE IF EXISTS insumo_costo;
DROP TABLE IF EXISTS insumo;
