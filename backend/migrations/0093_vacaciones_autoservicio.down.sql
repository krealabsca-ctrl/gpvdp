-- Revierte el autoservicio de vacaciones.
--
-- SE PIERDE LO QUE ESTABA EN CURSO: las solicitudes que no llegaron a confirmarse. Las vacaciones
-- ya confirmadas sobreviven intactas —son las que la tabla guardaba desde siempre— y los saldos no
-- se mueven, porque al volver atrás el filtro por estado desaparece y vuelve a contar «todo lo no
-- anulado», que es exactamente lo que queda.
--
-- Por eso las SOLICITADAS y APROBADAS se borran en vez de dejarse: sin la columna `estado` serían
-- indistinguibles de días disfrutados y se descontarían del saldo de gente que no se tomó un día.

BEGIN;

DELETE FROM vacacion WHERE estado IN ('SOLICITADA', 'APROBADA', 'RECHAZADA');

ALTER TABLE vacacion
    DROP COLUMN IF EXISTS estado,
    DROP COLUMN IF EXISTS solicitada_por,
    DROP COLUMN IF EXISTS aprobada_por,
    DROP COLUMN IF EXISTS aprobada_en,
    DROP COLUMN IF EXISTS confirmada_por,
    DROP COLUMN IF EXISTS confirmada_en,
    DROP COLUMN IF EXISTS motivo_rechazo;

DROP INDEX IF EXISTS idx_vacacion_estado;
DROP INDEX IF EXISTS idx_vacacion_fecha;

DROP TABLE IF EXISTS rrhh_jefatura;

DROP INDEX IF EXISTS idx_empleado_usuario;
ALTER TABLE empleado DROP COLUMN IF EXISTS usuario_id;

-- Los permisos salen de rol_permiso por la cascada de permiso.
DELETE FROM permiso WHERE codigo IN ('rrhh.vacaciones.mias', 'rrhh.vacaciones.equipo');

COMMIT;
