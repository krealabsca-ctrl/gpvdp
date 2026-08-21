-- Separar «lo decidí» de «nadie lo tocó» en la naturaleza del concepto.
--
-- La migración 0060 agregó `naturaleza NOT NULL DEFAULT 'NEUTRO'`. El default resolvió lo urgente
-- (que un concepto nuevo no entre al EBITDA sin que nadie lo decida) pero dejó las dos situaciones
-- guardadas en el MISMO byte:
--
--   · «Ahorro» es NEUTRO porque el usuario decidió que no es ingreso ni gasto. Correcto.
--   · «Patologías» es NEUTRO porque nadie abrió nunca el Catálogo. Falta decidir.
--
-- Medido al escribir esto: 42 conceptos en NEUTRO, 3 en INGRESO y 3 en GASTO — y esos 6 los puso la
-- propia 0060 por el nombre, ningún usuario. Es decir: hoy NO hay una sola declaración humana
-- registrada y el sistema no puede saberlo.
--
-- Dos consecuencias que se ven en pantalla:
--
--  1. El aviso del tablero («N conceptos sin declarar») cuenta `naturaleza = 'NEUTRO'`, así que
--     declarar «Ahorro» como NEUTRO —la respuesta CORRECTA— no lo baja. La única forma de apagar el
--     aviso es meter al EBITDA algo que no debe entrar: el aviso empuja al error que la 0060 vino a
--     corregir.
--  2. El selector del Catálogo muestra «— No entra» ya seleccionado en un concepto que nadie tocó:
--     le informa al usuario una decisión que nadie tomó.
--
-- Esta columna NO cambia ningún número: la naturaleza sigue siendo la misma y las expresiones del
-- EBITDA (`sqlIngresoNeto`, `sqlGastoNeto`, `sqlFueraDelEbitda`) preguntan por la naturaleza, no por
-- esta bandera. Lo único que agrega es poder decir «esto falta decidir».
ALTER TABLE concepto
    ADD COLUMN IF NOT EXISTS naturaleza_declarada boolean NOT NULL DEFAULT false;

COMMENT ON COLUMN concepto.naturaleza_declarada IS
    'true = una persona declaró la naturaleza (aunque haya elegido NEUTRO). false = nadie la declaró y el valor viene del default. Separa la decisión del silencio: sin esto, «no entra al EBITDA a propósito» y «falta decidir» son el mismo dato.';

-- Los conceptos raíz «Ingresos» y «Gastos» sí tienen su naturaleza fuera de duda (la puso la 0060
-- por el nombre y no hay otra lectura posible): quedan como declarados para que el aviso no pida
-- confirmar lo obvio. Todo lo demás nace SIN declarar, que es la verdad.
UPDATE concepto
   SET naturaleza_declarada = true
 WHERE naturaleza IN ('INGRESO', 'GASTO')
   AND lower(nombre) IN ('ingresos', 'gastos')
   AND NOT naturaleza_declarada;
