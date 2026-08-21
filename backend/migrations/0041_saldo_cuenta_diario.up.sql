-- Bancos — SALDO DIARIO por cuenta bancaria (Tanda 1, maqueta aprobada por el usuario).
--
-- Necesidad real de la operación: «la tesorera todos los días actualiza los saldos de las
-- cuentas para saber con cuánto contamos para el flujo de caja». Hasta hoy el sistema no
-- guardaba ningún saldo bancario (se revisó todo el esquema: las únicas columnas «saldo» eran
-- de nómina), así que no podía responder «cuánto tenemos hoy» ni demostrar que un mes está
-- COMPLETO — solo que se cargaron movimientos.
--
-- Se guarda únicamente el HECHO que declara una persona: el saldo que leyó en el banco. El
-- saldo esperado y la diferencia NO se guardan: se derivan de los movimientos cargados, así
-- que cuando entran los movimientos que faltaban la diferencia se cierra sola y no queda un
-- dato viejo contradiciendo la realidad (mismo criterio que los saldos de vacaciones).
--
-- El saldo va en la MONEDA DE LA CUENTA, sin convertir: mezclar colones y dólares exigiría un
-- tipo de cambio del día, que es una decisión pendiente del Director Financiero.

CREATE TABLE saldo_cuenta_diario (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id         uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    cuenta_bancaria_id uuid NOT NULL REFERENCES cuenta_bancaria(id),
    fecha              date NOT NULL,
    saldo              numeric(16, 2) NOT NULL,
    nota               text,
    capturado_por      uuid REFERENCES usuario(id),
    capturado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en     timestamptz NOT NULL DEFAULT now(),
    -- Un saldo por cuenta y día: recapturar corrige el del día, no agrega otro.
    UNIQUE (empresa_id, cuenta_bancaria_id, fecha)
);

-- El acceso natural es «los saldos de una fecha» (pantalla del día) y «la serie de una
-- cuenta» (posición de tesorería de los últimos días).
CREATE INDEX idx_saldo_diario_fecha ON saldo_cuenta_diario (empresa_id, fecha);
CREATE INDEX idx_saldo_diario_cuenta ON saldo_cuenta_diario (empresa_id, cuenta_bancaria_id, fecha DESC);
