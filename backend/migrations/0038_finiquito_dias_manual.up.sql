-- RRHH / Nómina — corrección tras revisión adversarial de la Etapa 4.
--
-- El finiquito precarga el saldo pendiente de vacaciones, pero al aprobar se reenviaban
-- esos mismos días como si RRHH los hubiera digitado: si el empleado disfrutó vacaciones
-- entre crear y aprobar, se congelaban días ya tomados (doble pago). Con esta bandera el
-- borrador distingue el saldo automático (se recalcula al aprobar) del valor que RRHH
-- escribió a mano (se respeta tal cual).
ALTER TABLE finiquito ADD COLUMN dias_vacaciones_manual boolean NOT NULL DEFAULT true;
COMMENT ON COLUMN finiquito.dias_vacaciones_manual IS
    'true = los días los digitó RRHH y se respetan · false = vienen del saldo y se recalculan al aprobar';
