-- ============================================================================
-- CxC · Arreglos de pago y lista de contacto preventivo
-- ----------------------------------------------------------------------------
-- Reglas que dio el negocio:
--   · Plazos ideales 1-3-6-9 cuotas. Puede haber excepciones.
--   · Los plazos largos los aprueba el SUPERVISOR DE PISO.
--   · Si el cliente incumple, aplica la regla de los 18 meses y el contrato pasa
--     a CARTERA MOROSA.
--   · El contacto preventivo (antes del vencimiento) va como lista aparte y con
--     su propio permiso.
--
-- Cómo se modeló, y por qué:
--
-- 1. El arreglo NO reescribe los cargos. Los cargos vencidos siguen vencidos con
--    su fecha original, así que la mora, el tramo, el aging y los 18 meses no se
--    borran por firmar un papel. El arreglo es un PLAN DE PAGOS encima de la
--    deuda: dice cuánto se compromete a pagar y cuándo. Los cobros se siguen
--    aplicando FIFO como siempre — el motor de aplicación no cambia ni una línea.
--
-- 2. El cumplimiento NO se guarda: se deriva de los cobros, igual que las
--    promesas. Y se mide ACUMULADO («a hoy debía haber pagado ₡X, pagó ₡Y»), no
--    cuota por cuota: quien adelanta la cuota 3 no aparece en mora en la 2.
--
-- 3. Solo un arreglo vivo por contrato. Dos planes simultáneos sobre la misma
--    deuda no se pueden juzgar: ¿de cuál era la cuota que pagó?
--
-- 4. Romper el arreglo es una DECISIÓN de una persona, con motivo. «En mora» lo
--    calcula el sistema (es un hecho); «quebrado» lo declara alguien (es una
--    consecuencia), igual que la suspensión.
-- ============================================================================

CREATE TABLE arreglo_pago_cxc (
    id                     uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id             uuid NOT NULL REFERENCES empresa (id) ON DELETE CASCADE,
    contrato_id            uuid NOT NULL REFERENCES contrato_cxc (id),
    consecutivo            bigint NOT NULL,

    -- La foto de la deuda al momento de pactar. Sin esto el arreglo no se puede
    -- juzgar después, porque el saldo cambia con cada cobro.
    saldo_al_pactar        numeric(18, 2) NOT NULL,
    vencido_al_pactar      numeric(18, 2) NOT NULL,
    cuotas_vencidas_al_pactar int NOT NULL DEFAULT 0,
    meses_mora_al_pactar   numeric(6, 1) NOT NULL DEFAULT 0,

    -- Lo pactado.
    monto_arreglo          numeric(18, 2) NOT NULL CHECK (monto_arreglo > 0),
    plazo_cuotas           int NOT NULL CHECK (plazo_cuotas >= 1),
    -- Prima o abono de entrada. Opcional: si no hubo, va en cero y no cambia nada.
    prima                  numeric(18, 2) NOT NULL DEFAULT 0 CHECK (prima >= 0),

    -- es_excepcion: el plazo no está en la lista estándar y lo tuvo que autorizar
    -- el supervisor de piso. Se guarda quién y por qué.
    es_excepcion           boolean NOT NULL DEFAULT false,
    autorizado_por         uuid REFERENCES usuario (id),
    autorizacion_motivo    text NOT NULL DEFAULT '',

    -- Cierre. «Al día» y «en mora» NO están acá: se derivan de los cobros.
    quebrado_en            timestamptz,
    quebrado_por           uuid REFERENCES usuario (id),
    quebranto_motivo       text NOT NULL DEFAULT '',
    anulado_en             timestamptz,
    anulado_por            uuid REFERENCES usuario (id),
    anulacion_motivo       text NOT NULL DEFAULT '',

    observaciones          text NOT NULL DEFAULT '',
    creado_por             uuid REFERENCES usuario (id),
    creado_en              timestamptz NOT NULL DEFAULT now(),

    UNIQUE (empresa_id, consecutivo)
);

CREATE INDEX idx_arreglo_cxc_contrato ON arreglo_pago_cxc (contrato_id, creado_en DESC);
CREATE INDEX idx_arreglo_cxc_empresa ON arreglo_pago_cxc (empresa_id, creado_en DESC);

-- Un solo arreglo vivo por contrato.
CREATE UNIQUE INDEX idx_arreglo_cxc_vivo ON arreglo_pago_cxc (contrato_id)
    WHERE quebrado_en IS NULL AND anulado_en IS NULL;

-- El plan, cuota por cuota. La cuota 0 es la prima (si hubo).
CREATE TABLE arreglo_cuota_cxc (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa (id) ON DELETE CASCADE,
    arreglo_id  uuid NOT NULL REFERENCES arreglo_pago_cxc (id) ON DELETE CASCADE,
    numero      int NOT NULL CHECK (numero >= 0),
    vence_en    date NOT NULL,
    monto       numeric(18, 2) NOT NULL CHECK (monto > 0),
    UNIQUE (arreglo_id, numero)
);

CREATE INDEX idx_arreglo_cuota_arreglo ON arreglo_cuota_cxc (arreglo_id, vence_en);

-- ── Parámetros ──────────────────────────────────────────────────────────────
-- ARREGLO_PLAZOS_ESTANDAR: lo que un gestor puede pactar solo. Cualquier otro
-- plazo es excepción y necesita el permiso del supervisor de piso.
INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, 'ARREGLO_PLAZOS_ESTANDAR', '1,3,6,9',
       'Plazos en cuotas que un gestor puede pactar sin autorización; cualquier otro plazo es excepción y lo aprueba el supervisor de piso'
FROM empresa e
ON CONFLICT DO NOTHING;

-- ARREGLO_PLAZO_MAXIMO no es una regla del negocio: es un guardarraíl de dato
-- para que un dedazo no genere un plan de 999 cuotas. El negocio lo puede mover.
INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, 'ARREGLO_PLAZO_MAXIMO', '60',
       'Tope duro de cuotas de un arreglo (guardarraíl contra errores de captura, no una regla de negocio)'
FROM empresa e
ON CONFLICT DO NOTHING;

-- DIAS_CONTACTO_PREVENTIVO: cuántos días antes del vencimiento entra un contrato
-- a la lista preventiva. Arranca en 7 (una semana antes) y es editable.
INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, 'DIAS_CONTACTO_PREVENTIVO', '7',
       'Días antes del vencimiento en que un contrato al día entra a la lista de contacto preventivo'
FROM empresa e
ON CONFLICT DO NOTHING;

-- ── Permisos ────────────────────────────────────────────────────────────────
-- El contacto preventivo es una lista aparte y con su propio permiso, como pidió
-- el negocio: llamar a quien todavía no debe nada es otra actividad que cobrar.
INSERT INTO permiso (codigo, modulo, nombre, descripcion)
VALUES ('cxc.preventivo', 'Cuentas por cobrar', 'Contacto preventivo',
        'Ver y gestionar la lista de contratos al día cuya cuota está por vencer (recordatorio antes del vencimiento)')
ON CONFLICT (codigo) DO NOTHING;

-- rol_permiso lleva empresa_id: la matriz de permisos es POR EMPRESA.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT e.id, r.id, p.id
FROM empresa e
CROSS JOIN rol r
JOIN permiso p ON p.codigo = 'cxc.preventivo'
WHERE r.codigo IN ('ADMIN', 'DIRECTOR_FINANCIERO', 'SUPERVISOR_FINANCIERO',
                   'AUXILIAR_FINANCIERO', 'SUPERVISOR_PISO')
ON CONFLICT DO NOTHING;

-- La descripción del permiso de suspensión hablaba de cuotas; la regla son meses.
UPDATE permiso
SET descripcion = 'Cortar el servicio a un contrato que llegó al tope de meses de mora, y reactivarlo'
WHERE codigo = 'cxc.suspender';
