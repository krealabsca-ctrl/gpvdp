-- Un documento que NO debe salir por el banco, aunque su estado lo permita.
--
-- Nace de un hueco medido en la fase 3 de inventario: la provisión de consignación que el sistema
-- genera al usar un cofre del proveedor recorría el camino completo hasta el archivo de pagos. Las
-- tres consultas que lo arman filtran únicamente por `estado = 'PROGRAMADO'`, y la provisión es de
-- tipo INTERNO —vía expresa—, así que se aprueba directo desde RECIBIDO. Si alguien la pagaba antes
-- de conciliarla, después entraba la factura electrónica REAL del mismo proveedor y se pagaba el
-- mismo cofre DOS VECES.
--
-- El Director Financiero eligió el diseño de «provisión que se concilia» precisamente porque tenía
-- «riesgo de doble pago cero». Esta columna es lo que cumple esa promesa.
--
-- Por qué una marca POSITIVA y no una lista de tipos que se excluyen: negar tipo por tipo obliga a
-- que cada consulta futura del archivo de pago se acuerde de la lista, y basta que una se olvide.
-- Además el default en FALSE deja las 4.542 facturas existentes exactamente como están: la columna
-- solo afecta a lo que se marque a propósito.
ALTER TABLE documento_cxp
    ADD COLUMN IF NOT EXISTS bloqueado_para_pago boolean NOT NULL DEFAULT false,
    ADD COLUMN IF NOT EXISTS bloqueo_motivo text;

COMMENT ON COLUMN documento_cxp.bloqueado_para_pago IS
    'true = no puede programarse ni entrar al archivo de pagos, aunque su estado lo permitiría. Lo '
    'usan las provisiones generadas por el sistema (consignación de inventario), que se reemplazan '
    'por la factura real del proveedor en vez de pagarse.';

COMMENT ON COLUMN documento_cxp.bloqueo_motivo IS
    'Por qué está bloqueado, en palabras, para que quien lo encuentre en la bandeja sepa qué hacer.';

-- Índice parcial: la pregunta que se hace es «¿cuáles están bloqueados?», y son unos pocos frente al
-- total. Indexar la columna entera desperdiciaría el índice en los miles de FALSE.
CREATE INDEX IF NOT EXISTS idx_documento_cxp_bloqueado
    ON documento_cxp (empresa_id, proveedor_id)
    WHERE bloqueado_para_pago;
