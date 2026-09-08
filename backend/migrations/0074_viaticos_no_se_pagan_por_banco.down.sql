-- Quitar el candado a los viáticos los vuelve programables y pagables por banco, que es lo que la
-- 0073 vino a impedir. Solo se desmarcan los que la 0073 marcó: se reconocen por el motivo, así que
-- un viático bloqueado a mano con otro motivo se queda como está.
UPDATE documento_cxp
   SET bloqueado_para_pago = false,
       bloqueo_motivo = NULL
 WHERE tipo = 'VIATICOS'
   AND bloqueado_para_pago
   AND bloqueo_motivo LIKE 'los viáticos no se pagan por banco%';
