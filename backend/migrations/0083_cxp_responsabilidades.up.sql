-- RESPONSABILIDADES MENSUALES (13 de setiembre de 2026).
--
-- EL PROBLEMA, MEDIDO. De 14 partidas de obligación con débito en el banco, 12 tienen CERO facturas
-- en Cuentas por pagar: Operaciones Bancarias ₡15,9 M/mes, Póliza ₡2,6 M, Mantenimiento de Carrozas
-- ₡1,5 M, Póliza de Riesgo de Trabajo ₡1,46 M, Alquiler ₡1,3 M, Internet ₡772 K, Telefonía ₡434 K.
-- Cerca de ₡28 millones por mes salen del banco sin que nadie los vea venir.
--
-- LA CAUSA ES ESTRUCTURAL, no descuido: los 4.542 documentos de CxP traen clave de Hacienda, sin una
-- sola excepción. La clave es obligatoria, así que un acuerdo sin factura —el arrendante de palabra,
-- el convenio por correo— HOY NO PUEDE EXISTIR en el sistema. Por eso esa plata sale por fuera.
--
-- Y el olvido también está medido: de 188 proveedores que facturan casi todos los meses, 126 (67 %)
-- se saltaron al menos un mes. La CNFL aparece 4 de 7 meses en una empresa con varias sedes.
--
-- LA FORMA. Todo el ERP hoy conoce solo LO QUE LLEGÓ. Esto es lo primero que declara LO QUE SE
-- ESPERA. El molde no se inventa: ya existe del lado de cobrar, en `cargo_cxc`, que es
-- `contrato_id + periodo` con un UNIQUE que hace que generar dos veces el mismo mes no duplique
-- nada. Acá se espeja invirtiendo el sentido.

-- ---------------------------------------------------------------------------
-- 1. EL ACUERDO. Se declara una vez y dura años.
-- ---------------------------------------------------------------------------
CREATE TABLE responsabilidad_cxp (
  id           uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  empresa_id   uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,

  nombre       text NOT NULL CHECK (btrim(nombre) <> ''),

  -- La contraparte va en TEXTO y el proveedor es opcional: el arrendante de palabra no está —ni
  -- tiene por qué estar— en el maestro de proveedores. Exigir proveedor_id reproduciría el mismo
  -- candado de la clave de Hacienda que es la causa del problema.
  contraparte   text NOT NULL CHECK (btrim(contraparte) <> ''),
  proveedor_id  uuid REFERENCES proveedor(id),

  -- PAGO: la consecuencia de no hacerlo es plata. TRAMITE: hay que presentar o renovar algo cuyo
  -- incumplimiento cuesta plata (una declaración, una póliza que se cae).
  -- Frontera dura: si la consecuencia NO es plata, no va acá. Una inspección de bomberos o una
  -- certificación no son cuentas por pagar, y si a los seis meses la mitad de la lista son cosas
  -- así, el módulo se equivocó de casa.
  tipo          text NOT NULL DEFAULT 'PAGO' CHECK (tipo IN ('PAGO', 'TRAMITE')),

  periodicidad  text NOT NULL DEFAULT 'MENSUAL'
                CHECK (periodicidad IN ('MENSUAL','BIMENSUAL','TRIMESTRAL','SEMESTRAL','ANUAL')),
  -- Día del mes en que vence. Medido: hay día fijo y es propiedad de la contraparte (SERVICIOS
  -- INTEGRADOS el 1 los 7 meses, TELEVISORA días 27-30; desviación de 0 a 1,7 días).
  -- El 31 en un mes de 30 lo resuelve el generador cayendo al último día, no esta columna.
  dia_vencimiento smallint NOT NULL CHECK (dia_vencimiento BETWEEN 1 AND 31),
  -- Para las que no son mensuales: en qué mes del año cae la primera del ciclo (1-12).
  mes_ancla       smallint CHECK (mes_ancla BETWEEN 1 AND 12),

  moneda          text NOT NULL DEFAULT 'CRC' CHECK (moneda IN ('CRC','USD')),
  monto_esperado  numeric(16,2) NOT NULL DEFAULT 0 CHECK (monto_esperado >= 0),
  -- FIJO vs VARIABLE no es cosmético: decide si el sistema puede algún día preparar el pendiente
  -- solo. Medido: los de consumo varían entre 21 % y 67 % de un mes a otro (PETRÓLEOS DELTA 67 %,
  -- SERVESMED 65 %). En los VARIABLE el monto es referencia para el flujo de caja y NUNCA autoriza
  -- un pago.
  monto_tipo      text NOT NULL DEFAULT 'FIJO' CHECK (monto_tipo IN ('FIJO','VARIABLE')),

  -- El respaldo se muestra EN LA FILA, no escondido en el detalle: un acuerdo de ₡2,7 millones al
  -- mes cuyo único respaldo es de palabra tiene que verse como tal de un vistazo.
  respaldo_tipo    text NOT NULL DEFAULT 'NINGUNO'
                   CHECK (respaldo_tipo IN ('CONTRATO','ACTA','CORREO','VERBAL','NINGUNO')),
  respaldo_archivo text NOT NULL DEFAULT '',

  -- Declara de entrada si va a llegar comprobante. Es lo que permite tratar el acuerdo sin factura
  -- sin inventarle un camino especial.
  espera_factura   boolean NOT NULL DEFAULT true,

  -- Decisión del Director (13-set-2026): cuando no hay comprobante, el gasto se marca NO DEDUCIBLE.
  -- En Costa Rica un gasto sin comprobante no lo acepta Hacienda, y una pantalla que normalice
  -- «respaldo: verbal» sin decirlo estaría empujando a registrar gasto que después se cae.
  -- El CHECK de abajo lo hace imposible de olvidar. Precedente: el vale de caja chica.
  deducible        boolean NOT NULL DEFAULT true,

  -- La partida es el EJE DE LOS PERMISOS (ver `rol_clasificacion_consulta`). Acá se declara a mano,
  -- así que es confiable: en las facturas miente (hay un garaje clasificado como «Agua») y 164 de
  -- los 188 proveedores mensuales están sin clasificar.
  clasificacion_id uuid REFERENCES clasificacion(id),
  departamento_id  uuid REFERENCES departamento(id),

  -- Nunca se borra: una responsabilidad borrada se lleva con ella la explicación de los meses que
  -- ya cerró. SUSPENDIDA la saca del calendario (por eso suspender es permiso crítico: permite
  -- tapar el olvido en vez de cumplirlo) y FINALIZADA la cierra para siempre.
  estado        text NOT NULL DEFAULT 'ACTIVA' CHECK (estado IN ('ACTIVA','SUSPENDIDA','FINALIZADA')),
  motivo_estado text NOT NULL DEFAULT '',

  notas         text NOT NULL DEFAULT '',
  creado_por    uuid REFERENCES usuario(id),
  creado_en     timestamptz NOT NULL DEFAULT now(),
  actualizado_en timestamptz NOT NULL DEFAULT now(),

  -- Sin comprobante no hay deducción posible: que la base lo garantice, no la pantalla.
  CONSTRAINT responsabilidad_sin_respaldo_no_deducible
    CHECK (respaldo_tipo NOT IN ('VERBAL','NINGUNO') OR deducible = false),
  -- Si se declara que existe un papel, tiene que estar adjunto. Decisión del Director: archivo
  -- obligatorio para poder pagar.
  CONSTRAINT responsabilidad_respaldo_con_archivo
    CHECK (respaldo_tipo NOT IN ('CONTRATO','ACTA','CORREO') OR btrim(respaldo_archivo) <> ''),
  -- Suspender o finalizar exige explicación escrita.
  CONSTRAINT responsabilidad_estado_con_motivo
    CHECK (estado = 'ACTIVA' OR btrim(motivo_estado) <> ''),
  -- Una responsabilidad no mensual necesita saber en qué mes arranca su ciclo.
  CONSTRAINT responsabilidad_ancla_si_no_es_mensual
    CHECK (periodicidad = 'MENSUAL' OR mes_ancla IS NOT NULL)
);

CREATE INDEX idx_responsabilidad_empresa ON responsabilidad_cxp (empresa_id, estado);
CREATE INDEX idx_responsabilidad_partida ON responsabilidad_cxp (empresa_id, clasificacion_id)
  WHERE estado = 'ACTIVA';
CREATE UNIQUE INDEX uq_responsabilidad_nombre
  ON responsabilidad_cxp (empresa_id, lower(btrim(nombre)))
  WHERE estado <> 'FINALIZADA';

-- ---------------------------------------------------------------------------
-- 2. QUIÉN LA LLEVA. Titular y suplente.
-- ---------------------------------------------------------------------------
-- Un solo responsable reproduce el problema que este módulo existe para resolver: el olvido ocurre
-- justamente cuando la persona no está. Con dos personas llevando toda la operación financiera, el
-- suplente no es un lujo — es lo que hace que el recordatorio sobreviva a unas vacaciones.
CREATE TABLE responsabilidad_responsable (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  empresa_id         uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
  responsabilidad_id uuid NOT NULL REFERENCES responsabilidad_cxp(id) ON DELETE CASCADE,
  usuario_id         uuid NOT NULL REFERENCES usuario(id),
  papel              text NOT NULL CHECK (papel IN ('TITULAR','SUPLENTE')),
  creado_en          timestamptz NOT NULL DEFAULT now(),
  -- Un titular y un suplente por responsabilidad, no una lista difusa donde nadie es el dueño.
  UNIQUE (responsabilidad_id, papel),
  -- Y la misma persona no puede ser titular Y suplente de lo mismo (sería un suplente de sí misma).
  UNIQUE (responsabilidad_id, usuario_id)
);

CREATE INDEX idx_responsabilidad_responsable_usuario
  ON responsabilidad_responsable (empresa_id, usuario_id);

-- ---------------------------------------------------------------------------
-- 3. EL MES. Una fila por responsabilidad y período.
-- ---------------------------------------------------------------------------
CREATE TABLE responsabilidad_periodo (
  id                 uuid PRIMARY KEY DEFAULT gen_random_uuid(),
  empresa_id         uuid NOT NULL REFERENCES empresa(id) ON DELETE CASCADE,
  responsabilidad_id uuid NOT NULL REFERENCES responsabilidad_cxp(id),

  periodo    text NOT NULL CHECK (periodo ~ '^[0-9]{4}-(0[1-9]|1[0-2])$'),
  vence_en   date NOT NULL,

  -- CONGELADO a propósito: si el alquiler sube en octubre, setiembre conserva lo que regía cuando
  -- se abrió. Un período que cambia de monto hacia atrás vuelve incomparable el histórico.
  monto_esperado numeric(16,2) NOT NULL DEFAULT 0 CHECK (monto_esperado >= 0),
  moneda         text NOT NULL DEFAULT 'CRC' CHECK (moneda IN ('CRC','USD')),

  estado text NOT NULL DEFAULT 'PENDIENTE'
         CHECK (estado IN ('PENDIENTE','CUMPLIDA','NO_APLICA')),

  -- CÓMO se cumplió. El semáforo de la pantalla (AL DÍA / POR VENCER / VENCIDA / SIN DATO) NO se
  -- guarda: se calcula al mirar, porque depende de la fecha de hoy y de hasta dónde está importado
  -- el banco. Un estado guardado envejece mal y miente al día siguiente.
  cumplida_con    text CHECK (cumplida_con IN ('FACTURA','MOVIMIENTO','ACUSE')),
  documento_id    uuid REFERENCES documento_cxp(id),
  movimiento_id   uuid REFERENCES movimiento_bancario(id),
  acuse_archivo   text NOT NULL DEFAULT '',

  -- «No aplica» sin motivo escrito no se guarda. Sin esto, «no aplica» se vuelve el botón de tapar
  -- el olvido — que es exactamente lo que este módulo existe para impedir.
  motivo     text NOT NULL DEFAULT '',

  cerrado_por uuid REFERENCES usuario(id),
  cerrado_en  timestamptz,
  creado_en   timestamptz NOT NULL DEFAULT now(),

  -- LA IDEMPOTENCIA, espejo exacto de cargo_cxc (contrato_id, periodo): abrir dos veces el mismo
  -- mes no duplica nada. Es el candado que permite que «Abrir el mes» sea un botón sin miedo.
  UNIQUE (responsabilidad_id, periodo),

  CONSTRAINT periodo_no_aplica_con_motivo
    CHECK (estado <> 'NO_APLICA' OR btrim(motivo) <> ''),
  -- Cumplida exige decir CÓMO y traer la prueba correspondiente.
  CONSTRAINT periodo_cumplida_con_prueba CHECK (
    estado <> 'CUMPLIDA' OR (
      (cumplida_con = 'FACTURA'    AND documento_id  IS NOT NULL) OR
      (cumplida_con = 'MOVIMIENTO' AND movimiento_id IS NOT NULL) OR
      (cumplida_con = 'ACUSE'      AND btrim(acuse_archivo) <> '')
    )
  ),
  -- Y al revés: una que no está cumplida no puede arrastrar pruebas de nada.
  CONSTRAINT periodo_pendiente_sin_prueba CHECK (
    estado = 'CUMPLIDA' OR (cumplida_con IS NULL AND documento_id IS NULL AND movimiento_id IS NULL)
  )
);

-- Una misma factura no puede cerrar dos responsabilidades, y un mismo débito bancario tampoco.
-- Sin esto, el alquiler y la póliza podrían «cumplirse» las dos con el mismo movimiento y el mes
-- cerraría en verde con una sola cosa pagada.
CREATE UNIQUE INDEX uq_periodo_documento ON responsabilidad_periodo (documento_id)
  WHERE documento_id IS NOT NULL;
CREATE UNIQUE INDEX uq_periodo_movimiento ON responsabilidad_periodo (movimiento_id)
  WHERE movimiento_id IS NOT NULL;

CREATE INDEX idx_periodo_empresa_periodo ON responsabilidad_periodo (empresa_id, periodo);
CREATE INDEX idx_periodo_abierto ON responsabilidad_periodo (empresa_id, vence_en)
  WHERE estado = 'PENDIENTE';
CREATE INDEX idx_periodo_responsabilidad ON responsabilidad_periodo (responsabilidad_id, periodo DESC);

-- ---------------------------------------------------------------------------
-- 4. PERMISOS
-- ---------------------------------------------------------------------------
-- Cinco, separados a propósito. DECLARAR es una decisión financiera que se toma una vez y compromete
-- a la empresa todos los meses; ABRIR EL MES es rutina. Juntarlos daría a quien opera la rutina el
-- poder de comprometer plata.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
  ('cxp.responsabilidades.ver', 'Cuentas por pagar', 'Ver las responsabilidades',
   'Ve todas las responsabilidades mensuales de la empresa y el estado del mes'),
  ('cxp.responsabilidades.ver_mias', 'Cuentas por pagar', 'Ver solo mis responsabilidades',
   'Ve únicamente aquellas donde la persona es titular o suplente. Se asigna persona por persona'),
  ('cxp.responsabilidades.declarar', 'Cuentas por pagar', 'Declarar y editar responsabilidades',
   'PERMISO CRÍTICO: crear, suspender, cambiar monto, día o responsable. Declara a qué se compromete la empresa todos los meses'),
  ('cxp.responsabilidades.abrir_mes', 'Cuentas por pagar', 'Abrir el mes de responsabilidades',
   'Genera los períodos del mes a partir de las responsabilidades activas. Operación mensual de Contabilidad'),
  ('cxp.responsabilidades.cerrar', 'Cuentas por pagar', 'Marcar cumplida o no aplica',
   'Enlazar la factura o el débito que prueba el cumplimiento, adjuntar el acuse, o marcar que el mes no aplicaba con su motivo')
ON CONFLICT (codigo) DO NOTHING;

-- Ver las responsabilidades va con quien ya ve el tablero de CxP: es la misma lectura del módulo.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT rp.empresa_id, rp.rol_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'cxp.dashboard'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo = 'cxp.responsabilidades.ver'
ON CONFLICT DO NOTHING;

-- Declarar, abrir el mes y cerrar arrancan SOLO en los roles que ya aprueban pagos. El resto se
-- asigna a mano desde la matriz: quién declara a qué se compromete la empresa es decisión del
-- Director, no un efecto colateral de una migración.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT rp.empresa_id, rp.rol_id, nuevo.id
FROM rol_permiso rp
JOIN permiso viejo ON viejo.id = rp.permiso_id AND viejo.codigo = 'cxp.aprobar'
CROSS JOIN permiso nuevo
WHERE nuevo.codigo IN ('cxp.responsabilidades.declarar',
                       'cxp.responsabilidades.abrir_mes',
                       'cxp.responsabilidades.cerrar')
ON CONFLICT DO NOTHING;

-- `cxp.responsabilidades.ver_mias` NO se asigna a nadie a propósito: es el permiso que responde
-- textualmente a «no todo mundo debe tener acceso a ver todas las responsabilidades», y quién lo
-- recibe lo decide el Director, persona por persona.
