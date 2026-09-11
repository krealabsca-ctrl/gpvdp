-- Revierte los permisos propios de las acciones en lote de CxP.
--
-- Al volver atrás, el código autoriza otra vez por código de rol, así que estos permisos dejan de
-- consultarse: se sacan del catálogo y `rol_permiso` los suelta por su FK en cascada.
--
-- Lo que NO se puede revertir es la marca que alguien le haya puesto a un rol a medida: si en la
-- matriz se le dio `cxp.anular` a «Supervisor Contable», al volver atrás ese rol pierde la acción
-- —no porque se borre el dato, sino porque el código vuelve a preguntar por el nombre del rol—.
DELETE FROM permiso WHERE codigo IN ('cxp.anular', 'cxp.resultado_pago');

UPDATE permiso SET descripcion = 'Marcar como revisadas' WHERE codigo = 'cxp.revisar';
