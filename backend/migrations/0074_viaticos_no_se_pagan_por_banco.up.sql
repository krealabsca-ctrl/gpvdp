-- Los viáticos no se pagan por banco, y hasta ahora nada lo impedía.
--
-- El tipo VIATICOS existe desde el principio con el comentario «no genera pago», y el estado
-- LIQUIDADA está documentado como «viáticos/almuerzos ya pagados: se archivan sin pago». Son gastos
-- que alguien YA desembolsó —de caja chica o de su bolsillo— y que se archivan, no se programan.
--
-- Pero las tres consultas que arman el archivo de pagos filtran únicamente por
-- `estado = 'PROGRAMADO'` y ninguna mira el tipo. Un viático que llegara a PROGRAMADO habría salido
-- al banco como cualquier factura, y el proveedor —o el empleado— habría recibido el dinero dos
-- veces: una en efectivo y otra por transferencia.
--
-- A partir de la migración 0072 hay un mecanismo para esto: `bloqueado_para_pago`. El service ya lo
-- pone al crear un VIATICOS nuevo; esta migración cubre los que ya estuvieran cargados en cualquier
-- base (en la de desarrollo hay 0, pero producción es otra instancia y no se puede asumir).
--
-- Solo toca los que TODAVÍA podrían pagarse: uno ya PAGADO o CONCILIADO se deja como está, porque
-- marcarlo ahora no desharía nada y sí ensuciaría el histórico.
UPDATE documento_cxp
   SET bloqueado_para_pago = true,
       bloqueo_motivo = COALESCE(NULLIF(bloqueo_motivo, ''),
           'los viáticos no se pagan por banco: ya se desembolsaron, y se archivan con «liquidar» en vez de programarse')
 WHERE tipo = 'VIATICOS'
   AND NOT bloqueado_para_pago
   AND estado NOT IN ('PAGADO', 'CONCILIADO', 'ANULADO', 'DENEGADO', 'LIQUIDADA');
