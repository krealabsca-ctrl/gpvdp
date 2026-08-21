ALTER TABLE cxc_suspension DROP COLUMN IF EXISTS meses_mora;

UPDATE cxc_parametro
SET clave = 'CUOTAS_PARA_SUSPENDER',
    descripcion = 'Cuotas vencidas sin pagar a partir de las cuales el contrato queda listo para suspender el servicio'
WHERE clave = 'MESES_PARA_SUSPENDER';
