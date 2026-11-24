-- Revierte la 0111. El orden importa: primero lo que depende, después lo que lo sostiene.

DROP TRIGGER IF EXISTS trg_linea_cuenta_de_movimiento ON asiento_linea;
DROP TRIGGER IF EXISTS trg_asiento_cuadra_lin ON asiento_linea;
DROP TRIGGER IF EXISTS trg_asiento_cuadra_cab ON asiento;
DROP FUNCTION IF EXISTS fn_linea_cuenta_de_movimiento();
DROP FUNCTION IF EXISTS fn_asiento_cuadra_lin();
DROP FUNCTION IF EXISTS fn_asiento_cuadra_cab();
DROP FUNCTION IF EXISTS fn_verificar_asiento(uuid);

DROP TABLE IF EXISTS asiento_linea;
DROP TABLE IF EXISTS asiento;
DROP TABLE IF EXISTS conta_consecutivo;

DROP TRIGGER IF EXISTS trg_mapeo_cb_cuenta_de_movimiento ON mapeo_cuenta_bancaria;
DROP TABLE IF EXISTS mapeo_cuenta_bancaria;
-- fn_mapeo_cuenta_de_movimiento() NO se borra: es de la 0110 y mapeo_contable la sigue usando.

DELETE FROM rol_permiso WHERE permiso_id IN (
    SELECT id FROM permiso WHERE codigo IN ('conta.bancos', 'conta.libros', 'conta.asientos'));
DELETE FROM permiso WHERE codigo IN ('conta.bancos', 'conta.libros', 'conta.asientos');
