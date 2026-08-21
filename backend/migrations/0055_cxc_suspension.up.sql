-- ============================================================================
-- CxC · SUSPENSIÓN POR MORA: 18 cuotas vencidas sin pagar
-- ----------------------------------------------------------------------------
-- Regla del negocio (confirmada por el usuario el 2026-08-05): el servicio se
-- suspende a las **18 cuotas vencidas sin pagar**. No son 18 meses de calendario
-- ni 18 meses desde el último pago: son 18 CUOTAS que vencieron y no se pagaron.
--
-- El modelo de partidas abiertas ya sabe contarlas sin agregar ni una columna: son
-- los cargos con `vence_en` en el pasado y `monto > monto_aplicado`. Como todo lo
-- demás en este módulo, el número se DERIVA — así un pago que entra hoy baja el
-- contador en el momento, sin que nadie recalcule nada.
--
-- Ojo con las quincenales: 18 cuotas quincenales son 9 meses de atraso, mientras
-- que 18 mensuales son 18 meses. La regla se implementa literal (18 cuotas) porque
-- es lo que el usuario dijo; si para quincenal debiera ser otro número, se cambia
-- el parámetro o se agrega uno por modalidad.
--
-- `contrato_cxc.estado` ya aceptaba 'SUSPENDIDO' y 'LEGAL' desde la migración
-- 0047 y NADA los escribía: era uno de los huecos de la auditoría. Ahora sí.
-- ============================================================================

INSERT INTO cxc_parametro (empresa_id, clave, valor, descripcion)
SELECT e.id, 'CUOTAS_PARA_SUSPENDER', '18',
       'Cuotas vencidas sin pagar a partir de las cuales el contrato queda listo para suspender el servicio'
FROM empresa e
ON CONFLICT DO NOTHING;

-- Rastro de la suspensión. No se borra al reactivar: queda la historia de que este
-- contrato estuvo cortado y por qué, que es justo lo que se pregunta después.
CREATE TABLE cxc_suspension (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    contrato_id uuid NOT NULL REFERENCES contrato_cxc(id) ON DELETE CASCADE,
    -- La foto del momento: cuántas cuotas debía y cuánto. Igual que en la gestión,
    -- «¿cuánto debía cuando le cortamos?» no se puede reconstruir después.
    cuotas_vencidas   int NOT NULL,
    saldo_al_suspender numeric(14, 2) NOT NULL DEFAULT 0,
    motivo      text NOT NULL,
    suspendido_por uuid REFERENCES usuario(id),
    suspendido_en  timestamptz NOT NULL DEFAULT now(),
    -- Reactivación: se llena cuando el contrato vuelve al servicio.
    reactivado_por    uuid REFERENCES usuario(id),
    reactivado_en     timestamptz,
    reactivacion_motivo text NOT NULL DEFAULT ''
);
CREATE INDEX idx_cxc_suspension_contrato ON cxc_suspension (contrato_id, suspendido_en DESC);
-- Un contrato no puede tener dos suspensiones vigentes a la vez.
CREATE UNIQUE INDEX idx_cxc_suspension_vigente ON cxc_suspension (contrato_id)
    WHERE reactivado_en IS NULL;

-- El índice que hace baratas las dos preguntas nuevas («cuántas cuotas vencidas
-- tiene este contrato» y «cuáles llegaron al tope») ya existe: el parcial
-- (empresa_id, contrato_id) INCLUDE (vence_en, monto, monto_aplicado) de la
-- migración 0052 resuelve el conteo con un Index Only Scan.
