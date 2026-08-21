-- Fase 2 (CxP): departamento del proveedor. Segmento adicional al "gasto predeterminado":
-- permite agrupar el gasto por área de la empresa (Logística, Mercadeo, Ventas, Cobros, Finanzas…)
-- para reportes y filtros. Vocabulario controlado desde el frontend; texto libre en DB.

ALTER TABLE proveedor ADD COLUMN departamento text;
