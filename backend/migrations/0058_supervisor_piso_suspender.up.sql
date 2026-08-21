-- ============================================================================
-- El Supervisor de Piso también corta el servicio por mora
-- ----------------------------------------------------------------------------
-- Decisión del usuario (2026-08-06): sí, el supervisor de piso puede suspender.
-- Es coherente con lo que ya autorizaba —notas de crédito sin tope y arreglos a
-- plazo largo—: es la autoridad de la operación de cobro, y suspender es la otra
-- cara de la misma decisión (o negocia un arreglo, o corta).
--
-- Se concede acá y no solo en la matriz por defecto para no depender de que
-- alguien corra `POST /rbac/permisos/aplicar-faltantes` en cada empresa.
--
-- Aparte se cerró un hueco que este cambio dejó a la vista: SUPERVISOR_PISO
-- existía únicamente en la migración 0054 y NO en `rbac.MatrizDefault`, así que
-- una empresa nueva nacía con el rol y CERO permisos, y `aplicar-faltantes`
-- nunca se los iba a dar porque solo recorre esa matriz. Ya está agregado ahí.
-- ============================================================================

-- Solo cxc.suspender. NO se le da `cxc.ver_todas_sedes`: «supervisor de PISO» nombra
-- a la autoridad de una plaza concreta, así que su alcance de datos sigue siendo
-- la(s) sede(s) que le asignen en Parámetros › Sedes y accesos. Si el negocio
-- quiere que vea toda la cartera, es una decisión aparte y se marca desde la UI.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT e.id, r.id, p.id
FROM empresa e
CROSS JOIN rol r
JOIN permiso p ON p.codigo = 'cxc.suspender'
WHERE r.codigo = 'SUPERVISOR_PISO'
ON CONFLICT DO NOTHING;

-- La descripción del rol decía solo notas y arreglos.
UPDATE rol
SET descripcion = 'Autoriza las excepciones de Cuentas por Cobrar: notas de crédito (sin tope), '
                  || 'arreglos de pago a plazo largo y la suspensión del servicio por mora'
WHERE codigo = 'SUPERVISOR_PISO';
