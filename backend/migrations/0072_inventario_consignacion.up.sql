-- Consignación: el proveedor deja el cofre en la funeraria y cobra cuando se usa.
--
-- La columna `es_consignada` existe desde 0068, pero no había forma de preguntar por ella sin
-- recorrer todas las unidades. La consignación se consulta SIEMPRE en un sentido —«qué del proveedor
-- salió de la bodega y no se le pagó»— y ese subconjunto es chico frente al total, así que el índice
-- va PARCIAL: indexa solo las consignadas en vez de todas las fichas.
CREATE INDEX IF NOT EXISTS idx_inv_unidad_consignada
    ON inv_unidad (empresa_id, estado, proveedor_id)
    WHERE es_consignada;

-- La factura que NACE al usar la unidad. Columna aparte de `documento_cxp_id` a propósito: esa
-- significa «la factura con la que la compré», y esta «la factura que se generó porque la usé».
-- Sobrecargar la primera con los dos sentidos es exactamente el defecto que corrigió la migración
-- 0070 con el estado NO_APARECIO: un hecho nuevo necesita su propio nombre, o el dato deja de poder
-- explicarse.
ALTER TABLE inv_unidad
    ADD COLUMN IF NOT EXISTS cxp_consignacion_id uuid REFERENCES documento_cxp(id);

COMMENT ON COLUMN inv_unidad.cxp_consignacion_id IS
    'Cuenta por pagar generada al usar esta unidad consignada. NULL = todavía no se le facturó al '
    'proveedor. El estado «pendiente de pago» se DERIVA de esta columna, no se guarda aparte.';

-- Una unidad no puede parir dos cuentas por pagar. El índice es parcial porque en Postgres NULL ≠
-- NULL: sin el WHERE, un UNIQUE dejaría pasar cualquier cantidad de unidades sin facturar, que es
-- justamente el caso normal. Cuarta vez que este proyecto se topa con eso (migs 0066, 0067, 0068).
CREATE UNIQUE INDEX IF NOT EXISTS uq_inv_unidad_cxp_consignacion
    ON inv_unidad (cxp_consignacion_id)
    WHERE cxp_consignacion_id IS NOT NULL;

COMMENT ON COLUMN inv_unidad.es_consignada IS
    'La unidad está en la bodega pero el capital es del proveedor: se le paga cuando se usa. Solo '
    'aplica a artículos de modo UNIDAD, porque los de cantidad no crean fichas y no habría dónde '
    'guardar de quién es cada objeto. Si es true, proveedor_id es obligatorio.';

COMMENT ON COLUMN inv_movimiento.proveedor_id IS
    'En una ENTRADA, de quién llegó la mercadería. En una SALIDA se llena solo cuando la unidad era '
    'consignada, y ahí es el dato operativo que dice a quién hay que pagarle por lo que se usó.';
