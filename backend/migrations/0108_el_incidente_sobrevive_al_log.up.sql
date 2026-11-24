-- 0107 · La referencia del incidente deja de vivir solo en el log.
--
-- ── LO QUE YA EXISTE, Y POR QUÉ NO ALCANZA ──────────────────────────────────────────────────
--
-- Desde el 12-oct `httpx.AbortInterno` genera una referencia de seis caracteres, se la devuelve
-- al usuario en el mensaje y la escribe en el log junto al error real. Eso ya resolvió lo peor:
-- «error interno» dejó de ser un callejón sin salida. 19 de las 22 salidas de 500 del sistema
-- pasan por ahí (medido por búsqueda en `internal/`, 22-oct-2026).
--
-- Lo que falta es dónde queda escrita. Hoy va a la salida estándar del contenedor: `docker logs`,
-- rotado. Para encontrar la referencia que un usuario dicta por teléfono hay que entrar por SSH
-- al servidor, y si el contenedor se reinició o el log ya rotó, no está. Es decir: el usuario
-- tiene un código que no se puede canjear por nada.
--
-- Y el panic es peor todavía. `gin.Recovery()` responde 500 SIN cuerpo: ni mensaje, ni
-- referencia. El stack queda en el log y el usuario ve una pantalla rota sin nada que dictar.
--
-- ── EL PEDIDO ───────────────────────────────────────────────────────────────────────────────
--
-- Punto 4 de la revisión comercial: «nadie se entera si se cae». Sin Sentry, sin OpenTelemetry,
-- sin Prometheus: solo logs en el servidor. Con un cliente propio es aceptable; con tres, te
-- enterás de los problemas por teléfono.
--
-- Decisión del Director Financiero (22-oct-2026): el registro vive ADENTRO del producto. La traza
-- de un error acá lleva montos, cédulas y nombres de proveedores, y mandarla a un servicio de
-- terceros es una decisión con consecuencias legales que se toma aparte, no de pasada.
--
-- ── QUÉ SE GUARDA Y QUÉ NO ──────────────────────────────────────────────────────────────────
--
-- Se guarda lo que permite encontrar y reproducir: la referencia, el método y la ruta, la
-- operación, el error real, quién y de qué empresa, y la traza SOLO cuando hubo panic.
--
-- NO se guarda el cuerpo de la petición. Ahí viajan contraseñas (el login), tokens de recepción
-- y datos de la fila que se estaba guardando. Un registro de errores que copia cuerpos es una
-- segunda base de datos sin control de acceso, y la primera vez que alguien la mire va a estar
-- leyendo la contraseña que otro tecleó mal.
--
-- `empresa_id` ADMITE NULO y es a propósito: un 500 en el login ocurre antes de que exista
-- empresa, y es justo el error que más importa ver. La pantalla muestra los de la empresa activa
-- más los que no tienen ninguna; nunca los de otra empresa.
--
-- La tabla CRECE y no se poda sola. Borrar por antigüedad en automático dejaría al sistema
-- tapando sus propios problemas sin que nadie lo decida; cuando haga falta, se poda a mano.

CREATE TABLE IF NOT EXISTS incidente (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    -- La referencia que el usuario tiene en pantalla. NO es única: seis hex se repiten cada
    -- tantos millones, y un UNIQUE haría fallar el registro del segundo incidente justo cuando
    -- hay muchos. Se busca por ella y se desempata por fecha.
    referencia  text        NOT NULL,
    empresa_id  uuid        REFERENCES empresa(id),
    usuario_id  uuid        REFERENCES usuario(id),
    metodo      text        NOT NULL,
    ruta        text        NOT NULL,
    estado      int         NOT NULL,
    operacion   text        NOT NULL DEFAULT '',
    detalle     text        NOT NULL DEFAULT '',
    panico      boolean     NOT NULL DEFAULT false,
    traza       text        NOT NULL DEFAULT '',
    creado_en   timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE incidente IS
  'Todo 5xx y todo panic, con la referencia que el usuario ve en pantalla. Append-only: nada lo '
  'actualiza. NO guarda el cuerpo de la petición (ahí viajan contraseñas y tokens).';
COMMENT ON COLUMN incidente.referencia IS
  'Los seis caracteres que el usuario dicta por teléfono (httpx.NuevaReferencia). No es única.';
COMMENT ON COLUMN incidente.empresa_id IS
  'Nulo cuando el error ocurre antes de elegir empresa (login, refresh). Esos se ven desde '
  'cualquier empresa; los de UNA empresa, solo desde ella.';
COMMENT ON COLUMN incidente.traza IS
  'Solo en panic. En un 500 normal el error ya está en `detalle` y la traza no agrega nada.';

-- Buscar por referencia es LA consulta: el usuario dicta seis caracteres y hay que encontrarlos.
CREATE INDEX IF NOT EXISTS ix_incidente_referencia ON incidente (referencia);
-- Y la otra: «qué se rompió últimamente», acotado a lo que esta empresa puede ver.
CREATE INDEX IF NOT EXISTS ix_incidente_empresa_fecha ON incidente (empresa_id, creado_en DESC);
CREATE INDEX IF NOT EXISTS ix_incidente_fecha ON incidente (creado_en DESC);

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- Va en «Administración», con `admin.correo`, `admin.plantillas` y `admin.roles`: es la misma
-- clase de acceso —la del que sostiene la instalación—, no una pantalla de trabajo.
--
-- Y tiene permiso propio en vez de colgarse de uno existente porque lo que muestra es distinto a
-- todo lo demás: rutas internas, nombres de tablas y fragmentos de SQL de los errores. Eso
-- describe el sistema por dentro, que es exactamente lo que sirve para atacarlo, y por eso el
-- mensaje de error nunca se le muestra crudo al usuario (ver httpx/interno.go).
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('admin.incidentes',
     'Administración',
     'Ver los errores del sistema',
     'Permite consultar los errores internos registrados y canjear la referencia que un usuario dicta por la causa exacta. Muestra detalles técnicos del sistema por dentro.')
ON CONFLICT (codigo) DO NOTHING;

-- Se siembra sobre las parejas (empresa, rol) que YA administran el sistema: quien puede tocar
-- roles y permisos es quien sostiene la instalación. No se le estrena la facultad a nadie.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso adm ON adm.id = rp.permiso_id AND adm.codigo = 'admin.roles'
CROSS JOIN permiso p
WHERE p.codigo = 'admin.incidentes'
ON CONFLICT DO NOTHING;
