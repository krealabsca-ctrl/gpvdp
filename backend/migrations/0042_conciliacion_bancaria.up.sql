-- Bancos — Conciliación bancaria mensual y congelamiento del saldo diario.
-- Decisiones del Director Financiero (2026-07-31):
--   1. El acta de conciliación es DOCUMENTO imprimible/firmable Y pantalla de control.
--   2. «Se debe cerrar todo e identificar todo»: el período NO cierra si alguna cuenta queda
--      con diferencia sin explicar (se suma a la regla de los «No identificado»).
--   3. El saldo capturado SE CONGELA cuando Dirección Financiera lo revisa: después nadie lo
--      edita sin dejar rastro.

-- ── 1. Congelamiento del saldo diario ────────────────────────────────────────
-- Revisado = congelado. La captura vuelve a rechazarse sobre una fila revisada (ver
-- GuardarSaldos): corregir un saldo congelado exige descongelarlo, y eso queda auditado.
ALTER TABLE saldo_cuenta_diario ADD COLUMN revisado_por uuid REFERENCES usuario(id);
ALTER TABLE saldo_cuenta_diario ADD COLUMN revisado_en  timestamptz;

-- ── 2. Partidas en tránsito de la conciliación ───────────────────────────────
-- Son los hechos que explican la diferencia entre lo que dice el banco y lo que dicen los
-- libros. El SIGNO no es libre: para los cuatro tipos conocidos lo fija el sistema (un mismo
-- hecho no puede entrar con signo distinto según quién lo capture); solo «OTRA» lo pide.
--   +1 = suma al saldo del banco para llegar al de libros
--   −1 = resta
CREATE TABLE partida_conciliacion (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id         uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    cuenta_bancaria_id uuid NOT NULL REFERENCES cuenta_bancaria(id),
    anio               int NOT NULL CHECK (anio >= 2024),
    mes                int NOT NULL CHECK (mes BETWEEN 1 AND 12),
    tipo               text NOT NULL CHECK (tipo IN (
                          'DEPOSITO_NO_ACREDITADO',      -- libros lo tiene, el banco todavía no
                          'TRANSFERENCIA_NO_PRESENTADA', -- se giró y el banco no la debitó
                          'CARGO_BANCO_NO_REGISTRADO',   -- el banco cobró y libros no lo registró
                          'ABONO_BANCO_NO_REGISTRADO',   -- el banco acreditó y libros no lo registró
                          'OTRA')),
    descripcion        text NOT NULL,
    monto              numeric(16, 2) NOT NULL CHECK (monto > 0),
    signo              smallint NOT NULL CHECK (signo IN (-1, 1)),
    -- Nunca se borra una partida: se anula y queda el rastro (regla del proyecto).
    anulada            boolean NOT NULL DEFAULT false,
    registrado_por     uuid REFERENCES usuario(id),
    registrado_en      timestamptz NOT NULL DEFAULT now(),
    anulada_por        uuid REFERENCES usuario(id),
    anulada_en         timestamptz
);
CREATE INDEX idx_partida_conc_cuenta_mes
    ON partida_conciliacion (empresa_id, cuenta_bancaria_id, anio, mes) WHERE NOT anulada;

-- ── 3. Firma del acta por cuenta y mes ───────────────────────────────────────
-- El acta se «cierra» cuando la diferencia sin explicar es cero y alguien la firma. El cierre
-- del período exige que TODAS las cuentas activas tengan su acta firmada.
CREATE TABLE acta_conciliacion (
    id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id         uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    cuenta_bancaria_id uuid NOT NULL REFERENCES cuenta_bancaria(id),
    anio               int NOT NULL,
    mes                int NOT NULL,
    -- Snapshot de las cifras al firmar: el acta es un documento, no una vista que cambia.
    saldo_banco        numeric(16, 2) NOT NULL,
    saldo_libros       numeric(16, 2) NOT NULL,
    ajuste_partidas    numeric(16, 2) NOT NULL,
    preparado_por      uuid REFERENCES usuario(id),
    preparado_en       timestamptz NOT NULL DEFAULT now(),
    firmado_por        uuid REFERENCES usuario(id),
    firmado_en         timestamptz,
    UNIQUE (empresa_id, cuenta_bancaria_id, anio, mes)
);
