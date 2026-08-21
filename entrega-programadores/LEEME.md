# Entrega técnica — GPVDP ERP

Carpeta para el equipo de programación. Todo lo que hay acá salió de **medir el sistema real**: el
esquema se extrajo de la base en operación y el catálogo de endpoints se generó del router. Ningún
número de estos documentos es una estimación.

**Corte:** 20 de agosto de 2026 · migración **0064** · **72 tablas** de negocio (más
`schema_migrations`, que crea golang-migrate) · **253 endpoints**

## Por dónde empezar

| Orden | Archivo | Qué es | Tamaño |
|---|---|---|---|
| 1º | **`MANUAL-TECNICO.md`** | El documento principal: arquitectura, cómo levantarlo, las reglas que no se pueden romper, cómo se cambia el esquema y las reglas de negocio que no se adivinan leyendo el código | 32 KB |
| 2º | **`schema.sql`** | El esquema completo de PostgreSQL, listo para correr sobre una base vacía | 153 KB |
| 3º | **`DICCIONARIO-DATOS.md`** | Las 72 tablas con sus 789 columnas: tipo exacto, obligatoriedad, valor por defecto, claves primarias y foráneas | 53 KB |
| 4º | **`ENDPOINTS.md`** | Las 253 rutas de la API con el permiso que exige cada una | 17 KB |

## Levantar la base en un paso

```bash
createdb -U postgres gpvdp
psql -U postgres -d gpvdp -f schema.sql
```

Queda la estructura completa: 72 tablas de negocio, 789 columnas, 231 índices, 189 claves foráneas.
Un `\dt` va a mostrar **73** tablas: la de más es `schema_migrations`, que crea golang-migrate para
llevar la cuenta de las migraciones aplicadas y no guarda datos del negocio.

**Probado el 20 de agosto de 2026**: se corrió este archivo sobre una base vacía y el resultado
coincide **exactamente** con la base real en las cuatro dimensiones (73 tablas · 791 columnas · 231
índices · 189 claves foráneas, contando `schema_migrations`).

> Antes de usar este camino para un ambiente real, leer la sección 4 del manual: para producción
> conviene dejar que el backend aplique las migraciones, porque así la base y el registro de
> migraciones quedan consistentes sin intervención manual.

## Tres cosas que conviene saber antes de tocar código

1. **El esquema se cambia con migraciones, no editando `schema.sql`.** La fuente de verdad son los
   64 archivos de `backend/migrations/`, que el backend aplica solo al arrancar. Ese archivo es una
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
