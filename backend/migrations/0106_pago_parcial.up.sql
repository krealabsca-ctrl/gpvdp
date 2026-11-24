-- 0105 · El pago parcial tiene salida: lo que el banco debitó de menos se registra y la factura
-- sigue debiendo el resto.
--
-- ── EL PROBLEMA (auditoría Bancos↔CxP, hallazgo 3.5) ────────────────────────────────────────
--
-- La conciliación exige coincidencia EXACTA, y eso está bien: en colones no hay diferencial que
-- justifique tolerancia. El problema es lo que pasa después. El veredicto `MONTO_DIFERENTE` es
-- TERMINAL: se cuenta en el reporte del barrido y no hay ninguna pantalla ni endpoint que lo
-- resuelva.
--
-- El propio código enumera las causas legítimas —«pago parcial, retención no aplicada, otra
-- factura»— y las deja sin camino. Cada una deja el movimiento sin enlazar y la factura sin
-- conciliar, para siempre.
--
-- ── POR QUÉ UNA TABLA DE APLICACIONES Y NO OTRA COLUMNA ─────────────────────────────────────
--
-- `movimiento_bancario.documento_cxp_id` tiene un índice ÚNICO parcial: hoy un solo movimiento
-- puede apuntar a cada factura, y está bien puesto —impide que dos débitos se declaren el pago de
-- la misma factura y la cuenten doble—.
--
-- Se podría haber quitado ese único para permitir varios movimientos por factura, pero eso rompe
-- algo que funciona: `documentoFrom` hace un LEFT JOIN contra el movimiento para mostrar con qué
-- se pagó cada factura, y con dos movimientos esa consulta devolvería la factura DUPLICADA en la
-- Bandeja. Es exactamente el defecto que la migración 0103 encontró al adjuntar la factura.
--
-- Por eso el enlace de siempre NO SE TOCA —un movimiento que paga una factura entera sigue
-- apuntándola por su columna— y lo parcial vive en su propia tabla, con el mismo molde que
-- `anticipo_aplicacion`: ya probado, y quien lea una entiende la otra.
--
-- ── UN DÉBITO PUEDE REPARTIRSE ENTRE VARIAS FACTURAS ────────────────────────────────────────
--
-- Decisión del Director Financiero (20-oct-2026). El caso que lo exige es la remesa que entró
-- corta: un pago agrupado cubre varias facturas, y si el banco debitó de menos hay que poder
-- decir cuánto de ese débito le tocó a cada una. Con una relación uno-a-uno habría que resolverlo
-- a mano fuera del sistema.
--
-- ── LAS TRES INVARIANTES ────────────────────────────────────────────────────────────────────
--
-- 1 · Lo aplicado a una FACTURA nunca supera su neto. Si no, se la daría por pagada de más.
-- 2 · Lo aplicado desde un MOVIMIENTO nunca supera su débito. Si no, el mismo colón se usaría dos
--     veces y el cuadre contra el banco dejaría de cerrar.
-- 3 · Un movimiento con `documento_cxp_id` o `remesa_id` NO puede tener aplicaciones parciales:
--     ya está aplicado entero. Son dos formas de decir lo mismo y hay que elegir una.
--
-- Las tres se verifican en el servicio, no con un CHECK: ninguna se puede expresar sobre UNA fila
-- —son sumas entre filas— y un CHECK que no puede verlas daría una falsa sensación de garantía.
-- Lo que el esquema sí garantiza está abajo.

CREATE TABLE IF NOT EXISTS pago_parcial (
    id             uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id     uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    documento_id   uuid NOT NULL REFERENCES documento_cxp(id),
    movimiento_id  uuid NOT NULL REFERENCES movimiento_bancario(id),

    -- En la moneda de la FACTURA, igual que el neto contra el que se compara. No se convierte
    -- entre monedas por la misma razón que en los anticipos (mig 0105, hallazgo 3.8): con qué
    -- tipo de cambio es una decisión contable que no está tomada.
    monto          numeric(16,2) NOT NULL CHECK (monto > 0),
    moneda         text NOT NULL DEFAULT 'CRC' CHECK (moneda IN ('CRC','USD')),

    -- OBLIGATORIO. Registrar un pago parcial es afirmar «el banco debitó menos que lo facturado y
    -- esto es lo que entró»; la diferencia puede ser una nota de crédito que todavía no llegó, una
    -- retención que no se aplicó, o un error del banco. Sin el motivo, dentro de seis meses nadie
    -- distingue cuál fue, y la factura queda con un saldo que nadie sabe explicar.
    motivo         text NOT NULL CHECK (length(btrim(motivo)) >= 20),

    aplicado_por   uuid REFERENCES usuario(id),
    aplicado_en    timestamptz NOT NULL DEFAULT now(),

    -- Se reversa, no se borra: es una tabla financiera. Igual que anticipo_aplicacion.
    activo         boolean NOT NULL DEFAULT true,
    reversado_por  uuid REFERENCES usuario(id),
    reversado_en   timestamptz
);

-- El MISMO movimiento no se aplica dos veces a la MISMA factura. Dos líneas así no son un reparto,
-- son un registro duplicado: lo que se quería era una sola línea por el total.
CREATE UNIQUE INDEX IF NOT EXISTS ux_pago_parcial_vivo
    ON pago_parcial (empresa_id, documento_id, movimiento_id)
    WHERE activo;

-- Las dos preguntas que se hacen a diario: «¿cuánto lleva pagado esta factura?» y «¿cuánto de
-- este débito ya está aplicado?». Las dos entran en el cálculo del neto, así que se consultan en
-- cada listado.
CREATE INDEX IF NOT EXISTS idx_pago_parcial_documento
    ON pago_parcial (empresa_id, documento_id) WHERE activo;
CREATE INDEX IF NOT EXISTS idx_pago_parcial_movimiento
    ON pago_parcial (empresa_id, movimiento_id) WHERE activo;

COMMENT ON TABLE pago_parcial IS
  'Lo que el banco debitó por una factura cuando NO alcanzó el neto. El neto a pagar resta estas '
  'aplicaciones, así que el saldo es lo que sale al banco en el próximo corte.';

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- Registrar el parcial va con `cxp.tesoreria`, el mismo con el que se concilia: es la misma
-- afirmación —este débito corresponde a esta factura— hecha por un monto que no calza entero.
--
-- VOLVER A COBRAR EL RESTO sí lleva permiso propio. No es lo mismo decir «entró esto» que decir
-- «el resto se sigue debiendo y hay que pagarlo»: si la diferencia era en realidad una nota de
-- crédito o una retención, cobrarla otra vez le paga de más al proveedor. Es el mismo criterio
-- que separó `cxp.liberar_pago` de `cxp.tesoreria`.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('cxp.cobrar_saldo',
     'Cuentas por pagar',
     'Mandar a cobrar el saldo de una factura pagada en parte',
     'Devuelve al circuito de pago lo que quedó debiendo una factura con pago parcial. Exige motivo: si la diferencia era una nota de crédito o una retención, volver a cobrarla le paga de más al proveedor.')
ON CONFLICT (codigo) DO NOTHING;

-- Sobre las parejas (empresa, rol) que YA existen: los roles base son globales (empresa_id NULL) y
-- `rol_permiso` lo exige NOT NULL. Lo recibe quien ya podía APROBAR el pago, que es la misma firma.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso apr ON apr.id = rp.permiso_id AND apr.codigo = 'cxp.aprobar'
CROSS JOIN permiso p
WHERE p.codigo = 'cxp.cobrar_saldo'
ON CONFLICT DO NOTHING;
