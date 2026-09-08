-- Quita la puerta de Contabilidad al catálogo de gasto.
--
-- No devuelve nada a cambio: quien tenía `cxp.catalogo` vuelve a no poder abrir rubros, que es
-- exactamente el estado anterior. El ON DELETE CASCADE de rol_permiso limpia las concesiones.
--
-- Los rubros YA creados por Contabilidad se quedan: son catálogo real con facturas clasificadas
-- colgando. Borrarlos sería perder la clasificación de esas facturas.
DELETE FROM permiso WHERE codigo = 'cxp.catalogo';
