-- Correo saliente POR EMPRESA + bitácora de los envíos del comprobante.
--
-- Dos huecos que hoy conviven:
--
-- (1) El servidor de correo es UNO SOLO para todo el grupo (SMTP_ADDR / SMTP_FROM / SMTP_USER /
--     SMTP_PASS, variables del proceso) y el mailer se arma UNA vez en el arranque
--     (cmd/api/main.go). Así, Coopeprofa le escribe a sus proveedores desde el buzón de Valle de
--     Paz. Cada empresa manda desde su propio correo empresarial —decisión del Director
--     Financiero, 17-set-2026— y eso no se resuelve con variables de entorno: es CONFIGURACIÓN
--     POR EMPRESA, igual que el texto de los correos (plantilla_correo, mig 0044).
--
-- (2) Del envío del comprobante solo queda `documento_cxp.comprobante_enviado_en`, y esa columna
--     se PISA con now() en cada envío. No dice a quién se le mandó, ni con copia a quién, ni si
--     el servidor lo aceptó. «Reenviar» sin bitácora es un botón sin memoria: nadie puede
--     contestar «¿cuándo y a qué dirección se le mandó el comprobante a este proveedor?».
--
-- Se sigue el molde de plantilla_correo (0044) y cxp_fuente_recepcion (0081): una fila por
-- empresa, lo que la empresa NO configuró no existe como fila (y entonces rige el global), y el
-- secreto nunca se guarda en claro.

-- ─────────────────────────────────────────────────────────────────────────────
-- 1. El correo saliente de cada empresa
-- ─────────────────────────────────────────────────────────────────────────────
--
-- UNA fila por empresa: «una empresa, un remitente». El UNIQUE sobre empresa_id no es
-- decorativo — es lo que hace que la resolución al enviar sea una sola lectura sin desempates.
--
-- LA CONTRASEÑA VA CIFRADA, NO HASHEADA. Es la diferencia con `cxp_fuente_recepcion.token_hash`
-- (0081), que es sha256 irreversible porque ahí solo hay que COMPARAR lo que llega. Acá hay que
-- PRESENTARLE la contraseña al servidor SMTP, así que tiene que poder volver a texto claro. Se
-- cifra con AES-256-GCM (backend/internal/shared/cifrado.go) y la clave vive FUERA de la base, en
-- la variable de entorno CIFRADO_SECRET: si alguien se lleva un respaldo de Postgres, se lleva
-- texto inútil.
--
-- ⚠ SI CIFRADO_SECRET SE PIERDE, LAS CONTRASEÑAS DE ESTA TABLA SON IRRECUPERABLES. No hay puerta
--   de atrás: hay que volver a pedirle a cada empresa su contraseña de aplicación y escribirla de
--   nuevo en la pantalla. Y si se ROTA sin re-cifrar, lo guardado deja de descifrar y el envío de
--   esa empresa responde 422 diciendo justamente eso — falla ruidosa, nunca caída silenciosa al
--   correo global (eso sería volver al bug que esta migración viene a arreglar, ahora sin que
--   nadie lo note).
--
-- El prefijo de versión («v1.») se guarda adentro del propio valor a propósito: el día que haya
-- que cambiar de algoritmo, se distingue lo viejo de lo nuevo sin adivinar ni migrar a ciegas.
CREATE TABLE correo_saliente (
    id               uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id       uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,

    -- El servidor
    host             text NOT NULL,
    puerto           int  NOT NULL DEFAULT 587,
    -- NINGUNO = sin cifrado (MailHog de desarrollo); STARTTLS = 587 (lo que hoy hace el código);
    -- TLS = 465, TLS implícito. OJO para quien implemente el envío: el 465 NO está soportado por
    -- `smtp.SendMail`, así que ofrecer este valor obliga a implementarlo en el mismo cambio; si no,
    -- el usuario elige una opción que no funciona y parece un problema de su servidor.
    seguridad        text NOT NULL DEFAULT 'STARTTLS',
    -- Usuario vacío = sin autenticación. Es el modo de MailHog y el único caso en que
    -- `password_cifrada` puede quedar vacía (ver correo_saliente_auth_completa).
    usuario          text NOT NULL DEFAULT '',
    -- El valor cifrado queda ATADO a (empresa_id, host, puerto, usuario): esas cuatro cosas entran
    -- como datos autenticados del AES-GCM, así que cambiar cualquiera vuelve este blob
    -- INDESCIFRABLE. No es una sutileza criptográfica, es el control que impide llevarse la
    -- contraseña sin verla nunca: sin él, quien pueda editar esta fila apunta el `host` a su propio
    -- servidor dejando el campo de contraseña vacío («ausente = se conserva la guardada»), aprieta
    -- «probar», y el sistema le presenta la contraseña real a ese servidor. Con el atado, no hay
    -- nada que entregar. (Hallazgo S-1 de la revisión de seguridad del 2026-09-17.)
    --
    -- CONSECUENCIA OBLIGATORIA PARA EL SERVICE: si el PUT cambia host, puerto, usuario o
    -- seguridad, la contraseña guardada se descarta y `password` pasa a ser OBLIGATORIA en ese
    -- mismo PUT (422 «cambió el servidor: hay que escribir de nuevo la contraseña»). Es una
    -- defensa de seguridad, no una validación de formulario: no se "simplifica".
    password_cifrada text NOT NULL DEFAULT '',

    -- Quién aparece como remitente
    remitente        text NOT NULL,
    remitente_nombre text NOT NULL DEFAULT '',

    -- Apagada (activo = false) la empresa vuelve al correo global del servidor SIN destruir la
    -- credencial guardada. Por eso no hay borrado: apagar es reversible, borrar obliga a volver a
    -- escribir la contraseña.
    activo           boolean NOT NULL DEFAULT true,

    -- Resultado de la última prueba de conexión. Es el equivalente del «latido» de 0081: sin esto,
    -- «nunca se probó» se ve igual que «se probó y funcionó». probado_error NULL con probado_en
    -- lleno = la última prueba salió bien.
    --
    -- ACÁ, Y SOLO ACÁ, va el DETALLE TÉCNICO del fallo (clasificado y saneado: nunca la
    -- contraseña). Esta tabla se lee detrás de `admin.correo`, que hoy tiene UN rol
    -- (DIRECTOR_FINANCIERO). La bitácora de abajo la leen SIETE roles y por eso allá solo va la
    -- categoría. (Hallazgo S-3.)
    probado_en       timestamptz,
    probado_error    text,

    actualizado_por  uuid REFERENCES usuario(id),
    actualizado_en   timestamptz NOT NULL DEFAULT now(),
    creado_en        timestamptz NOT NULL DEFAULT now(),

    UNIQUE (empresa_id),
    CONSTRAINT correo_saliente_seguridad_check CHECK (seguridad IN ('NINGUNO','STARTTLS','TLS')),
    CONSTRAINT correo_saliente_puerto_check    CHECK (puerto BETWEEN 1 AND 65535),
    -- Un remitente mal escrito no falla al guardar, falla al enviarle al proveedor: se ataja acá.
    CONSTRAINT correo_saliente_remitente_check CHECK (remitente ~ '^[^@[:space:]]+@[^@[:space:]]+\.[^@[:space:]]+$'),
    -- Guardarraíl de último recurso contra una contraseña en claro: lo cifrado SIEMPRE empieza con
    -- la marca de versión, así que un INSERT con texto plano rebota en la base.
    CONSTRAINT correo_saliente_password_formato CHECK (password_cifrada = '' OR password_cifrada LIKE 'v1.%'),
    -- Con usuario pero sin contraseña, el envío intentaría AUTH PLAIN con la contraseña vacía: el
    -- servidor rechaza, el correo no sale y nadie entiende por qué. Si hay usuario, hay secreto.
    CONSTRAINT correo_saliente_auth_completa CHECK (usuario = '' OR password_cifrada <> '')
);

-- ─────────────────────────────────────────────────────────────────────────────
-- 2. La bitácora de envíos del comprobante
-- ─────────────────────────────────────────────────────────────────────────────
--
-- UNA FILA POR ENVÍO, incluidos los que FALLARON. Que el envío falle es justamente lo que hoy no
-- deja rastro: el error se le muestra al usuario en pantalla y se pierde al recargar.
--
-- QUÉ PASA CON documento_cxp.comprobante_enviado_en: SE CONSERVA, no se deriva ni se borra.
-- Tres razones: (a) la leen la Bandeja (pestaña «Pagadas») y el detalle del documento a través
-- de `ListarDocumentos`/`DocumentoPorID`, y es una columna de la MISMA tabla que ya se selecciona
-- —convertirla en un subselect a esta bitácora le agrega un JOIN a la consulta más caliente del
-- módulo—; (b) responde otra pregunta que la bitácora: «¿el proveedor ya tiene el PDF que está
-- adjunto HOY?», mientras que la bitácora responde «¿qué se mandó y a quién, alguna vez?»;
-- (c) romperla obligaría a tocar pantallas que esta tarea no pide.
-- Lo que SÍ cambia es su significado, y se escribe acá para que no se vuelva a perder: pasa a ser
-- «fecha del último envío OK del comprobante que está adjunto ahora», y por eso el reemplazo del
-- adjunto la pone en NULL (arreglo en el repositorio de CxP, etapa 2).
--
-- NO SE HACE BACKFILL de los documentos que ya figuran como enviados. Sería inventar datos: no
-- sabemos a qué dirección se les mandó (el correo del proveedor pudo cambiar desde entonces), ni
-- quién lo mandó, ni con qué archivo. La bitácora arranca vacía y dice la verdad desde el día uno.
CREATE TABLE comprobante_envio (
    id            uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id    uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
    documento_id  uuid NOT NULL REFERENCES documento_cxp(id) ON DELETE CASCADE,

    -- A quién se le mandó. Se guarda el TEXTO, no el id del proveedor: el correo de la ficha puede
    -- cambiar mañana y la bitácora tiene que seguir diciendo a dónde fue el correo de ayer.
    destinatario  text NOT NULL,
    -- La copia al aprobador (decisión del DF, 17-set-2026). Vacío = la factura no tenía aprobador
    -- con correo; el envío NO se cae por eso, va sin copia y queda escrito acá.
    --
    -- Va OCULTA (BCCO) salvo que el Director decida lo contrario: publicarle a 649 proveedores
    -- externos la dirección de quien autoriza los pagos es entregar, factura por factura, el blanco
    -- exacto de un fraude de cambio de cuenta bancaria. Y con net/smtp la copia se entrega por el
    -- SOBRE (la lista de destinatarios), no por una cabecera `Bcc:` — escribir la cabecera la
    -- retransmiten muchos servidores (el proveedor ve lo que se quiso ocultar) y ponerla sin el
    -- sobre no entrega nada, con lo cual esta columna diría «copia» sobre una copia que nunca
    -- existió. (Hallazgo S-9.)
    copia         text NOT NULL DEFAULT '',
    remitente     text NOT NULL DEFAULT '',

    -- De dónde salió la configuración con la que se mandó: EMPRESA (correo_saliente) o GLOBAL
    -- (las variables del servidor). Es lo que permite ver de un vistazo qué empresas siguen
    -- mandando desde el buzón equivocado.
    origen        text NOT NULL DEFAULT 'EMPRESA',

    -- QUÉ archivo se mandó. `comprobante_subido_en` es el `subido_en` del adjunto en el momento
    -- del envío: con eso se distingue «se reenvió el mismo PDF» de «se mandó otro PDF».
    archivo                text NOT NULL DEFAULT '',
    comprobante_subido_en  timestamptz,

    resultado     text NOT NULL,

    -- POR QUÉ FALLÓ, EN DOS NIVELES DE DETALLE (hallazgo S-3).
    --
    -- Esta tabla se lee con `cxp.ver`, que hoy tienen SIETE roles; la configuración del correo se
    -- decidió como responsabilidad de UNO. Así que acá va solo lo que el operador de CxP necesita
    -- para saber qué hacer, y NADA del servidor: ni host, ni usuario del buzón, ni el texto crudo
    -- de la respuesta SMTP. El detalle técnico vive en `correo_saliente.probado_error`, detrás de
    -- `admin.correo`.
    --
    -- `error_categoria` es la clasificación que devuelve el transporte; `error` es la frase para el
    -- usuario («el servidor de correo rechazó las credenciales — avisar a Dirección»). La
    -- contraseña no entra a ninguna de las dos, nunca, ni aunque el servidor la repita en su propia
    -- respuesta: el transporte clasifica y el texto libre del servidor no cruza esa frontera.
    error_categoria text NOT NULL DEFAULT '',
    error           text,

    enviado_por   uuid REFERENCES usuario(id),
    enviado_en    timestamptz NOT NULL DEFAULT now(),

    CONSTRAINT comprobante_envio_resultado_check CHECK (resultado IN ('OK','ERROR')),
    CONSTRAINT comprobante_envio_origen_check    CHECK (origen IN ('EMPRESA','GLOBAL')),
    CONSTRAINT comprobante_envio_categoria_check CHECK (error_categoria IN
        ('','AUTENTICACION_RECHAZADA','HOST_INALCANZABLE','RELAY_DENEGADO','TLS_FALLIDO','TIEMPO_AGOTADO','CORREO_NO_CONFIGURADO','SECRETO_ILEGIBLE','OTRO')),
    -- Un OK no tiene categoría de error, y un ERROR sí: si no, «falló» y «salió bien» se
    -- distinguen solo por convención y la primera consulta que agrupe por causa miente.
    CONSTRAINT comprobante_envio_categoria_coherente CHECK (
        (resultado = 'OK'    AND error_categoria = '') OR
        (resultado = 'ERROR' AND error_categoria <> '')
    )
);
-- El orden descendente va en el índice porque la pantalla siempre pide «los últimos de este
-- documento» y nunca los primeros.
CREATE INDEX idx_comprobante_envio_doc ON comprobante_envio (empresa_id, documento_id, enviado_en DESC);

-- ─────────────────────────────────────────────────────────────────────────────
-- 3. El permiso nuevo, con la regla «nadie gana ni pierde»
-- ─────────────────────────────────────────────────────────────────────────────
--
-- Es UNO solo. Reenviar el comprobante y ver la bitácora NO estrenan permiso: reenviar es el
-- mismo acto que enviar (`cxp.comprobante`) y leer la bitácora es leer el expediente de la
-- factura (`cxp.ver`). Un permiso por botón es lo que vuelve inusable la matriz.
--
-- `critico = true`: guarda una credencial de un buzón corporativo. El catálogo de código
-- (internal/rbac/catalogo.go) lo vuelve a sembrar con los mismos metadatos en cada arranque; acá se
-- declara igual para que la migración quede completa por sí sola.
INSERT INTO permiso (codigo, modulo, nombre, descripcion, critico) VALUES
  ('admin.correo', 'Administración', 'Correo saliente',
   'Configurar el servidor de correo desde el que cada empresa envía (servidor, remitente y contraseña) y probar la conexión',
   true)
ON CONFLICT (codigo) DO NOTHING;

-- Va con quien ya decide el TEXTO de los correos que salen a nombre de la empresa
-- (`admin.plantillas`): es la misma responsabilidad —la voz de la empresa hacia afuera— y hoy la
-- tiene el Director Financiero. Guardar acá una contraseña de correo corporativo no es una tarea
-- de operación, así que NO se le da a Tesorería ni al Auxiliar.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT rp.empresa_id, rp.rol_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'admin.plantillas'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'admin.correo'
ON CONFLICT DO NOTHING;

-- NOTA: esta concesión es SOLO para las empresas ya instaladas. `rbac.EnsureDefaults` no reparte
-- permisos nuevos a una empresa que ya tiene matriz configurada (repository.go: si el rol ya tiene
-- alguna concesión, se salta). Para que una empresa NUEVA nazca con esto, el permiso está agregado
-- al catálogo de internal/rbac/catalogo.go; DIRECTOR_FINANCIERO lo hereda solo porque su matriz es
-- `codigos()`.
