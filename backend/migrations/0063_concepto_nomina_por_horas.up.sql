-- Marcar explícitamente qué conceptos de nómina se registran por HORAS.
--
-- Hasta ahora la pantalla lo decidía buscando la palabra «hora» en el NOMBRE del concepto. Si
-- alguien lo renombraba —«Tiempo extraordinario», «Extras»— el campo dejaba de pedir horas y
-- pasaba a pedir un monto: la captura seguía funcionando, pero el monto ya no se derivaba con el
-- factor de ley (art. 139 CT, mínimo 1,5) y la hora extra se pagaba mal SIN que nada fallara.
--
-- Con la bandera, el comportamiento depende de una decisión explícita y no de cómo se escribió
-- el nombre.
ALTER TABLE concepto_nomina
    ADD COLUMN IF NOT EXISTS por_horas boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN concepto_nomina.por_horas IS
    'true = la novedad se captura en HORAS y el monto lo deriva el motor (horas × valor hora × factor, art. 139 CT). false = se captura el monto del período.';

-- Conservar el comportamiento actual: lo que la pantalla venía tratando por horas sigue igual.
UPDATE concepto_nomina
   SET por_horas = true
 WHERE tipo = 'INGRESO'
   AND lower(nombre) LIKE '%hora%'
   AND por_horas IS DISTINCT FROM true;
