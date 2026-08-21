DELETE FROM rol_permiso
WHERE rol_id IN (SELECT id FROM rol WHERE codigo = 'SUPERVISOR_PISO')
  AND permiso_id IN (SELECT id FROM permiso WHERE codigo = 'cxc.suspender');

UPDATE rol
SET descripcion = 'Autoriza notas de crédito (sin tope) y arreglos de pago a plazo largo en Cuentas por Cobrar'
WHERE codigo = 'SUPERVISOR_PISO';
