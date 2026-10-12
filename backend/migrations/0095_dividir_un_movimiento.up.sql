-- Dividir un movimiento entre dos partidas, conservando el hecho bancario.
--
-- El caso, del Director Financiero (9-oct-2026): «del recibo de luz este mes se pagó 1.400.000,
-- pero resulta que de luz normal eran 1.000.000, y esos 400.000 de diferencia corresponden a un
-- proyecto especial de ese mes. Entonces del movimiento principal se quita lo del proyecto, pero
-- la sumatoria de ambas suman el movimiento real, y se conserva la integridad del consecutivo,
-- fecha, descripción.»
--
-- Es lo que los ERP grandes llaman «split» o líneas de distribución, y la regla es siempre la
-- misma: EL HECHO BANCARIO NO SE INVENTA NI SE PIERDE. Lo que se reparte es la atribución
-- contable; la suma de las partes tiene que dar exactamente lo que salió del banco.
--
-- POR QUÉ HIJOS Y NO UNA TABLA DE LÍNEAS. Lo ortodoxo sería una tabla `movimiento_linea` y que
-- todos los reportes sumaran líneas en vez de movimientos. Acá eso significaba reescribir 43
-- consultas repartidas en 52 archivos —el cuadre, el EBITDA, el tablero, las proyecciones, los
-- reportes— y cada una es una oportunidad de equivocarse con la plata. Con movimientos hijos, cada
-- parte YA es una fila clasificable y todo lo construido sigue funcionando sin tocarse: suman
-- montos, no cuentan filas.
--
-- Lo que se pierde es que un movimiento deja de ser 1:1 con una línea del estado de cuenta. Se
-- compensa con `dividido_de`: desde cualquier parte se llega al original, y la suma del padre más
-- sus hijos es, por construcción, el monto que trajo el banco.

BEGIN;

ALTER TABLE movimiento_bancario
    -- El hijo apunta al movimiento del que salió. NULL = no es una parte de nada.
    --
    -- ON DELETE no se declara a propósito: en tablas financieras no hay borrado físico, así que la
    -- cascada nunca debería dispararse. Si algún día alguien intenta borrar un padre, que falle.
    ADD COLUMN dividido_de  uuid REFERENCES movimiento_bancario(id),
    ADD COLUMN dividido_en  timestamptz,
    ADD COLUMN dividido_por uuid REFERENCES usuario(id),
    -- Por qué se separó esta parte. Va en el HIJO, que es la parte que alguien decidió apartar.
    ADD COLUMN dividido_motivo text;

COMMENT ON COLUMN movimiento_bancario.dividido_de IS
  'Movimiento del que esta fila es una parte. El padre conserva consecutivo, fecha y descripción, y su monto quedó reducido en lo que se apartó: padre + hijos = lo que trajo el banco.';

CREATE INDEX idx_mov_dividido_de ON movimiento_bancario (dividido_de)
    WHERE dividido_de IS NOT NULL;

-- `monto_antes_de_dividir` guarda lo que el banco trajo, SOLO en el padre y solo al dividirse.
--
-- El nombre es largo porque el corto ya está tomado: `monto_original` es el monto en la MONEDA
-- original de la cuenta —va con `moneda_original` y `tc_aplicado`, lo usan 31.193 filas y de él
-- sale el monto en colones—. Reusar ese nombre habría pisado el cálculo del tipo de cambio.
--
-- Es redundante con «padre + hijos» a propósito: esa suma es la invariante que el sistema
-- mantiene, y este campo es con qué comprobarla. Una invariante que solo se puede verificar
-- recalculándola con la misma fórmula que la produjo no se está verificando.
ALTER TABLE movimiento_bancario ADD COLUMN monto_antes_de_dividir numeric(16,2);

COMMENT ON COLUMN movimiento_bancario.monto_antes_de_dividir IS
  'Lo que trajo el banco antes de dividir, solo en el movimiento padre. Sirve para comprobar la invariante: padre + hijos = monto_antes_de_dividir. NO confundir con monto_original, que es el monto en la moneda de la cuenta.';

-- Los permisos. Dividir y deshacer la división son la MISMA autoridad —repartir el gasto de un
-- movimiento entre partidas— y van juntos en un permiso propio.
--
-- No se cuelga de `bancos.clasificar`: clasificar es decir a qué partida va un movimiento;
-- dividir es decidir que un movimiento son DOS hechos contables. Es una atribución más fuerte y
-- quien la tenga debería elegirse aparte.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('bancos.dividir_movimiento',
     'Bancos',
     'Dividir un movimiento entre partidas',
     'Separar parte de un movimiento a otra clasificación (un proyecto especial dentro del recibo de luz). La suma de las partes siempre da lo que trajo el banco')
ON CONFLICT (codigo) DO NOTHING;

-- Arranca en los roles que ya clasifican: es quien conoce las partidas y quien se topa con el
-- recibo que mezcla dos cosas. Se siembra desde las parejas (empresa, rol) que YA existen en
-- `rol_permiso` y no recorriendo `rol`, porque los roles base son globales (empresa_id NULL) y
-- `rol_permiso` exige la empresa.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'bancos.clasificar'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'bancos.dividir_movimiento'
ON CONFLICT DO NOTHING;

COMMIT;
