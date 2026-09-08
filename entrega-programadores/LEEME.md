# Entrega técnica — GPVDP ERP

Carpeta para el equipo de programación. Todo lo que hay acá salió de **medir el sistema real**: el
esquema se extrajo de la base en operación y el catálogo de endpoints se generó del router. Ningún
número de estos documentos es una estimación.

**Corte:** 3 de setiembre de 2026 · migración **0078** · **86 tablas** de negocio (más
`schema_migrations`, que crea golang-migrate) · **936 columnas** · **321 endpoints**

## Por dónde empezar

| Orden | Archivo | Qué es | Tamaño |
|---|---|---|---|
| 1º | **`MANUAL-TECNICO.md`** | El documento principal: arquitectura, cómo levantarlo, las reglas que no se pueden romper, cómo se cambia el esquema y las reglas de negocio que no se adivinan leyendo el código | 37 KB |
| 2º | **`schema.sql`** | El esquema completo de PostgreSQL, listo para correr sobre una base vacía | 188 KB |
| 3º | **`DICCIONARIO-DATOS.md`** | Las 86 tablas con sus 936 columnas: tipo exacto, obligatoriedad, valor por defecto, claves primarias y foráneas | 65 KB |
| 4º | **`ENDPOINTS.md`** | Las 321 rutas de la API con el permiso que exige cada una | 24 KB |

## Levantar la base en un paso

```bash
createdb -U postgres gpvdp
psql -U postgres -d gpvdp -f schema.sql
```

Queda la estructura completa: 86 tablas de negocio, 936 columnas, 286 índices, 250 claves foráneas.
Un `\dt` va a mostrar **87** tablas: la de más es `schema_migrations`, que crea golang-migrate para
llevar la cuenta de las migraciones aplicadas y no guarda datos del negocio.

**El archivo trae sellada la versión de las migraciones** (`INSERT INTO schema_migrations` al final,
hoy la 78). Eso importa: sin esa fila, el backend arranca leyendo versión NULL, cree que la base está
sin migrar e intenta aplicar la 0001 en adelante **sobre tablas que ya existen**. La primera falla, la
base queda marcada `dirty`, y el mensaje de error no dice nada de esto. Con la fila puesta, el backend
ve que el esquema ya está al día y no toca nada.

**Probado el 27 de agosto de 2026**: se restauró este archivo sobre una base vacía y quedaron las 85
tablas con `schema_migrations` en la versión 78 y `dirty = false`.

## Tres cosas que conviene saber antes de tocar código

1. **El esquema se cambia con migraciones, no editando `schema.sql`.** La fuente de verdad son los
   78 archivos de `backend/migrations/`, que el backend aplica solo al arrancar. Ese archivo es una
   foto para levantar y comparar. Manual, sección 9.

2. **Toda consulta filtra por `empresa_id`, y ese valor sale del token.** Nunca del cuerpo ni del
   query string. Un repositorio que no lo aplique es un bug de seguridad. Manual, sección 6.1.

3. **El dinero nunca es punto flotante.** `numeric` en la base, `decimal.Decimal` en Go, **string**
   en el JSON. Hoy el esquema no tiene ni una columna de punto flotante. Manual, sección 6.2.

## Lo que todavía no está resuelto

Dos cosas que el manual detalla y que hay que atender temprano:

- **No hay control de versiones.** El proyecto no tiene repositorio git. Es lo primero, porque sin
  eso no existe forma ordenada de que varias personas incorporen cambios. Sección 10, incluido el
  `.gitignore` mínimo (los respaldos de la base **no** entran al repositorio).
- **Limitaciones conocidas**, entregadas dichas y no escondidas: la auditoría no guarda el valor
  anterior, una importación bancaria no se puede deshacer, cerrar un período es irreversible y no
  hay límite de intentos de login. Sección 12, con dónde está cada una.

## Cómo regenerar estos documentos

Hay un script que lo hace, con el sistema levantado:

```bash
bash deploy/regenerar-entrega.sh
```

Qué hace y qué no, a propósito:

- **`schema.sql` se regenera completo** (`pg_dump --schema-only --no-owner --no-privileges`). Es
  mecánico: no hay nada escrito por una persona que se pueda perder.
- **`ENDPOINTS.md` y `DICCIONARIO-DATOS.md` se verifican, no se reescriben.** Los dos agrupan y
  explican con criterio humano —las secciones, las convenciones de tipos— y un generador lo
  destruiría. El script dice exactamente qué ruta o qué columna falta y **termina con error** si
  falta algo, así sirve de control antes de entregar.

Conviene correrlo como paso de la rutina de publicación. Un documento generado que nadie regenera
miente con más autoridad que uno escrito a mano: `schema.sql` sin una columna nueva se corre **sin
dar error** y la aplicación falla después, en otra pantalla, sin que nadie relacione las dos cosas.
Es exactamente lo que había pasado: el paquete estaba 6 rutas y 2 columnas atrás.
