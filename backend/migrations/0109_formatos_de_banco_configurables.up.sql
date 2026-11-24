-- 0108 · Agregar un banco deja de necesitar un programador.
--
-- ── EL PROBLEMA MEDIDO (punto 2 de la revisión comercial) ───────────────────────────────────
--
-- Los seis formatos de banco viven en `internal/bancos/specs.go`, en Go: columnas numeradas a
-- mano (`colFecha: 0, colDoc: 1, …`) y una función `isHeader` por banco. Cada banco nuevo es un
-- ticket de desarrollo, un build y un despliegue. Para Costa Rica eso es menos grave de lo que
-- suena —BN, BCR, BAC, Davivienda, Promerica y Banco Popular son casi el mercado—, pero es el
-- techo de cuántos clientes se pueden atender sin crecer el equipo.
--
-- ── LO QUE RESULTÓ SER DATO, Y LO QUE PARECÍA CÓDIGO ────────────────────────────────────────
--
-- De las cinco cosas que definen un formato, CUATRO ya son datos: el texto que identifica el
-- archivo, las cinco columnas y el formato de fecha. La única que es código es `isHeader`, y al
-- mirarlas las seis siguen el MISMO molde:
--
--     cellHas(c, 0, "fecha") && cellHas(c, 1, "documento") && cellHas(c, 3, "debito")
--
-- Es decir: «la celda N contiene el texto T», encadenado con Y. Eso cabe en una lista de pares
-- {columna, texto}, que es lo que guarda `senales`. Hasta el caso raro —Davivienda, que exige DOS
-- textos en la misma columna 3: "debitos" y "dr"— entra sin inventar nada.
--
-- ── LOS SEIS DE CÓDIGO NO SE TOCAN ──────────────────────────────────────────────────────────
--
-- Esta migración no migra nada: crea una tabla VACÍA. El importador prueba primero los formatos
-- configurados y, si ninguno calza, cae a los seis de siempre. Con la tabla vacía el sistema se
-- comporta EXACTAMENTE como hoy, que es la condición que puso el Director Financiero: «nada de lo
-- que hagamos puede romper lo que funciona».
--
-- Los configurados van primero y no al final porque el caso que viene es un banco que CAMBIA su
-- exportación: entonces el formato de código deja de calzar y hay que poder ponerle uno nuevo
-- delante sin esperar un despliegue. Si fueran al final, un formato de código a medias seguiría
-- ganando y el arreglo no serviría.
--
-- ── POR EMPRESA, NO GLOBAL ──────────────────────────────────────────────────────────────────
--
-- `empresa_id NOT NULL`, como todo lo demás. Un formato mal configurado puede hacer que un
-- archivo se lea con las columnas corridas —débitos donde van créditos—, y eso no puede cruzar de
-- una empresa a otra. El costo es recrearlo en cada empresa que use ese banco; es poco frecuente
-- y es el lado seguro del error.

CREATE TABLE IF NOT EXISTS formato_banco (
    id          uuid PRIMARY KEY DEFAULT gen_random_uuid(),
    empresa_id  uuid        NOT NULL REFERENCES empresa(id),
    -- El nombre que va a ver la gente y que queda escrito en `importacion.banco`.
    nombre      text        NOT NULL,
    activo      boolean     NOT NULL DEFAULT true,
    -- Texto que tiene que aparecer en alguna parte de las primeras filas («banco popular»).
    -- Opcional: tres de los seis formatos de código no lo usan y se reconocen solo por encabezado.
    texto_identificador text NOT NULL DEFAULT '',
    -- Columnas 0-based, igual que en los specs de código. -1 = la columna no existe en ese banco.
    col_fecha       int NOT NULL,
    col_documento   int NOT NULL,
    col_descripcion int NOT NULL,
    col_debito      int NOT NULL,
    col_credito     int NOT NULL,
    -- Clave de una lista CERRADA (ver formatoDeFecha en Go). No es un layout de Go: pedirle
    -- «01-02-06» a quien configura el sistema es pedirle que adivine una convención interna.
    formato_fecha   text NOT NULL,
    -- [{"columna":0,"texto":"fecha"}, …]. TODAS tienen que cumplirse (se unen con Y).
    senales         jsonb NOT NULL DEFAULT '[]'::jsonb,
    creado_por      uuid REFERENCES usuario(id),
    creado_en       timestamptz NOT NULL DEFAULT now(),
    actualizado_en  timestamptz NOT NULL DEFAULT now()
);

COMMENT ON TABLE formato_banco IS
  'Formatos de estado de cuenta configurados desde la pantalla. Se prueban ANTES que los seis que '
  'vienen en el código; con la tabla vacía el importador se comporta igual que siempre.';
COMMENT ON COLUMN formato_banco.senales IS
  'Las condiciones que identifican la fila de encabezado: [{"columna":N,"texto":"…"}]. Se unen '
  'con Y. El texto se compara normalizado (minúsculas, sin acentos) y por CONTENIDO, no exacto.';
COMMENT ON COLUMN formato_banco.col_documento IS
  'Columna de la referencia del banco. -1 cuando ese banco no publica ninguna.';

-- Dos formatos con el mismo nombre en la misma empresa no se pueden distinguir en la pantalla, y
-- el nombre es lo que queda escrito en cada importación. Se compara sin may/min: «BAC» y «bac»
-- son el mismo banco para quien lee la lista.
CREATE UNIQUE INDEX IF NOT EXISTS ux_formato_banco_nombre
    ON formato_banco (empresa_id, lower(nombre));

-- ── El permiso ──────────────────────────────────────────────────────────────────────────────
--
-- Propio y no `bancos.importar`: importar es la tarea diaria de quien carga los estados de
-- cuenta; configurar un formato es decidir CÓMO se leen las columnas de dinero de todos los
-- archivos que vengan después. Un formato con los débitos y los créditos cambiados ensucia la
-- contabilidad entera y nadie lo nota hasta el cierre.
INSERT INTO permiso (codigo, modulo, nombre, descripcion) VALUES
    ('bancos.formatos',
     'Bancos',
     'Configurar formatos de banco',
     'Permite enseñarle al sistema a leer el estado de cuenta de un banco nuevo: en qué columna viene cada dato y cómo se reconoce el archivo.')
ON CONFLICT (codigo) DO NOTHING;

-- Lo recibe quien ya puede tocar los parámetros de Bancos de la empresa (`bancos.ajustes`): es la
-- misma clase de decisión —cómo se comporta el módulo—, no una tarea de operación.
INSERT INTO rol_permiso (empresa_id, rol_id, permiso_id)
SELECT DISTINCT rp.empresa_id, rp.rol_id, p.id
FROM rol_permiso rp
JOIN permiso aj ON aj.id = rp.permiso_id AND aj.codigo = 'bancos.ajustes'
CROSS JOIN permiso p
WHERE p.codigo = 'bancos.formatos'
ON CONFLICT DO NOTHING;
