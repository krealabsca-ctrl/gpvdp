# PAQUETE DE ENTREGA TÉCNICA — Build Kit para Claude Code
### Finance Group GPVDP · v1.0 · Puente de la Especificación Funcional al Desarrollo

> **Regla de oro:** la especificación funcional (Bancos v1.0) es **necesaria pero no suficiente** para que Claude Code construya sin interpretar. Un LLM sin *control stack* produce código genérico. Este kit es la capa de control: reglas siempre activas, conocimiento bajo demanda, trabajadores especializados y verificación determinista.

---

## 0. Qué te falta para entregar (inventario del *gap*)

Tienes ✅ la spec funcional. Para el *handoff* completo faltan **siete** artefactos:

1. **Contratos técnicos** — Diccionario de datos (DDL a nivel de columna/tipo) + OpenAPI 3.1. Sin esto, Claude Code inventa nombres de campos y firmas de API.
2. **Control stack de Claude Code** — `CLAUDE.md` + subagentes (`.claude/agents/`) + skills nuevas (`.claude/skills/`) + hooks. Es lo que evita el desarrollo genérico.
3. **Andamiaje del repo + entorno local** — estructura monorepo, `docker-compose`, `Makefile`, seeds. Es lo que te permite **probar en tu local**.
4. **Estándares de código y de pruebas** — para que "todo se testee" y se entreguen "productos terminados" (Definition of Done).
5. **Sistema de diseño premium** — dirección de UI por encima de Hope UI (tokens, librerías, patrones de dashboard).
6. **Plan por fases con criterios de aceptación** — para "primero todo con lo que tenemos, luego ajustamos".
7. **Insumos de negocio pendientes** — [T1] formatos de banco, [T2] fórmula EBITDA, [T3] matriz de roles, + 2 decisiones (tolerancia de traslados, cierre bloqueante).

Este documento entrega 2–6 y precisa 1 y 7.

---

## 1. Cómo se controla Claude Code para que NO sea genérico

Cuatro capas, cada una con un trabajo distinto (verificado contra la doc oficial de Claude Code):

| Capa | Archivo | Cuándo carga | Para qué en GPVDP |
|---|---|---|---|
| **Memoria de proyecto** | `CLAUDE.md` (raíz) | En **cada** prompt | Stack fijo, convenciones, glosario mínimo, guardarraíles legales, multi-tenant. |
| **Skills** | `.claude/skills/<n>/SKILL.md` | **Bajo demanda** según la `description` | Conocimiento profundo: convenciones Go/Gin, dominio GPVDP, fiscal CR, adaptadores de banco. |
| **Subagentes** | `.claude/agents/<n>.md` | Cuando el trabajo matchea su `description` | Trabajadores aislados con su propio contexto: backend, frontend, DB, QA, seguridad. |
| **Hooks** | `.claude/settings.json` | Determinista (evento) | Forzar `gofmt`, `golangci-lint`, tests antes de commit; bloquear `.env`. |

Regla práctica: **reglas globales → `CLAUDE.md`**; **conocimiento específico de una tarea → skill**; **trabajo delegable en contexto propio → subagente**; **política que debe cumplirse siempre → hook**.

---

## 2. Tus skills actuales → cobertura y brechas

**Reutilizables directamente (fuerte cobertura):**

| Skill tuya | Cubre en GPVDP |
|---|---|
| `system-design`, `api-architect`, `database-architect` | Arquitectura, contratos, modelo PostgreSQL. |
| `ui-ux-pro-max` | Sistema de diseño premium (§9). |
| `tdd-workflow`, `frontend-testing` | Estrategia de pruebas (§10). |
| `security-audit` | Seguridad, RBAC, multi-tenant, auditoría. |
| `fintech-engineer`, `analisis-horizontal-financiero`, `generating-reports` | Dominio financiero, reportería, KPIs. |
| `hook-development`, `coding-agent`, `skill-creator` | Para construir el resto del control stack. |

**Brechas que debes cerrar antes del handoff (créalas con `skill-creator`):**

| Skill nueva | Por qué (brecha real) |
|---|---|
| **`go-gin-backend`** | 🔴 Crítica. Tu `backend-patterns` es **Node/Express/Next** y `backend-code-review` es **.py**. No hay convenciones de Go/Gin. Aquí van: arquitectura por capas (handler → service → repo), `pgx`/`sqlc`, `golang-migrate`, manejo de errores, middleware multi-tenant, validación con `go-playground/validator`. |
| **`gpvdp-domain`** | 🔴 Crítica para "no genérico". Glosario y reglas: Concepto vs Clasificación, memoria IBAN, congelamiento de TC (promedio 1/15/último), regla de duplicados (huella intra-archivo), huella Bancos↔CxP, No-identificado ≥90%, traslados/overnight. |
| **`cr-fiscal-compliance`** | 🔴 Crítica (legal). Reglas CR: comprobante electrónico 4.4, retenciones, CCSS 10.67% obrero / ~26.5% patronal, tramos de renta, **y el guardarraíl de nómina** (§4). Impide que un agente construya la función de "ahorro por no reportar". |
| **`multi-tenant-postgres`** | Alta. Patrón `empresa_id` en toda query, middleware que inyecta el tenant, índices `(empresa_id, fecha)`, particionamiento. Evita fugas entre empresas. |
| **`bank-import-adapters`** | Alta. Patrón *strategy* por banco: detección de inicio de movimientos, descarte de residuos, IBAN, CRC/USD, formatos con caracteres. Se nutre de [T1]. |
| **`gpvdp-data-layer`** (frontend) | Media. Tu `frontend-query-mutation` es **oRPC/Dify**; aquí el cliente se genera del **OpenAPI** (`openapi-typescript` + TanStack Query). Convención distinta. |

> Tip: `skill-creator` viene preinstalado en Claude Desktop/Cowork; en Claude Code se instala con `/plugin install skill-creator@anthropic-agent-skills`. Mantén cada `SKILL.md` < ~500 tokens y empuja el detalle a archivos `references/` que carguen bajo demanda.

---

## 3. Subagentes a crear (`.claude/agents/`)

Espejo del comité, pero como trabajadores con contexto aislado. Formato: Markdown con *frontmatter* `name`, `description`, `tools`, `model`. Los 5 esenciales para Fase 0–1:

**`.claude/agents/backend-go.md`**
```markdown
---
name: backend-go
description: Usar PROACTIVAMENTE para cualquier trabajo de backend Go+Gin del ERP GPVDP (handlers, services, repos, migraciones, endpoints). Devuelve código idiomático con tests.
tools: Read, Write, Edit, Bash, Glob, Grep
model: sonnet
---
Eres el Backend Tech Lead (Go + Gin) de GPVDP. Antes de escribir, lee la skill `go-gin-backend`, `gpvdp-domain` y `multi-tenant-postgres`.
Reglas: arquitectura por capas (handler→service→repository); nunca SQL en handlers; todo endpoint filtra por empresa_id del contexto; validación en el borde; errores tipados; sin lógica de negocio en Gin. Cada función pública lleva test de tabla. No inventes reglas de negocio: si falta una, DETENTE y pregunta.
```

**`.claude/agents/frontend-react.md`**
```markdown
---
name: frontend-react
description: Usar PROACTIVAMENTE para UI React+TS del ERP GPVDP (pantallas, tablas de datos, dashboards). Aplica el sistema de diseño premium.
tools: Read, Write, Edit, Bash, Glob, Grep
model: sonnet
---
Eres el Frontend Tech Lead. Lee las skills `ui-ux-pro-max` y `gpvdp-data-layer` antes de construir.
Stack: Vite + React + TS estricto, Tailwind + shadcn/ui, TanStack Query + Table + Virtual, Tremor/Recharts + ECharts (calendarios). Nada de "look de IA genérico" (evita Inter + gradiente morado por defecto). Estado de servidor solo con TanStack Query; cliente tipado generado del OpenAPI. Cada componente con test (Vitest + RTL).
```

**`.claude/agents/db-postgres.md`**
```markdown
---
name: db-postgres
description: Usar para diseño de esquema, migraciones y optimización PostgreSQL de GPVDP. Read-mostly salvo migraciones.
tools: Read, Write, Edit, Bash, Grep
model: sonnet
---
Eres el PostgreSQL Architect. Lee `database-architect` y `multi-tenant-postgres`.
Toda tabla transaccional lleva empresa_id + índices compuestos. Migraciones reversibles con golang-migrate. Nada de borrado físico en tablas financieras (soft-delete + auditoría append-only). Respeta el diccionario de datos (§8) al pie de la letra.
```

**`.claude/agents/qa-tdd.md`**
```markdown
---
name: qa-tdd
description: Usar PROACTIVAMENTE después de escribir código y antes de dar por terminado un entregable. Escribe y ejecuta pruebas; verifica el Definition of Done.
tools: Read, Write, Edit, Bash, Glob, Grep
model: sonnet
---
Eres el QA Lead. Lee `tdd-workflow` y `frontend-testing`.
Backend: tests de tabla + integración con testcontainers-postgres. Frontend: Vitest+RTL + E2E Playwright de los flujos críticos (importar → clasificar → cuadrar). Cobertura mínima 80% en dominio financiero. Un entregable no está "terminado" si algún caso del §10 falla.
```

**`.claude/agents/security-reviewer.md`**
```markdown
---
name: security-reviewer
description: Usar PROACTIVAMENTE antes de commits que toquen auth, permisos, multi-tenant, importaciones de archivos o exportaciones. Solo lectura.
tools: Read, Grep, Glob, Bash
model: sonnet
---
Eres el Cybersecurity Architect. Lee `security-audit`.
Verifica: aislamiento por empresa_id en cada query, RBAC por rol×acción×empresa, aprobación mancomunada donde aplique, saneamiento de archivos importados (terceros no confiables), MFA en exportación/congelamiento, cifrado en reposo/tránsito, auditoría inmutable. Reporta por severidad con archivo/línea.
```

> Los demás (BI/reportería, DevOps/infra, integraciones) se añaden en Fase 2+.

---

## 4. `CLAUDE.md` del repositorio (listo para pegar en la raíz)

```markdown
# GPVDP ERP — Memoria de Proyecto

## Qué es
ERP financiero multiempresa (Valle de Paz, Coopeprofa, Memorial Pets · escalable).
Multi-tenant con aislamiento por empresa. Este ERP es el sistema de registro (no hay contabilidad externa).

## Stack (NO desviarse sin aprobación)
- Backend: Go 1.26 + Gin · pgx/sqlc · golang-migrate · validator · JWT(+refresh)
- Frontend: Vite + React + TypeScript estricto · Tailwind + shadcn/ui · TanStack Query/Table/Virtual · Tremor/Recharts/ECharts
- DB: PostgreSQL 16
- Infra local: Docker Compose · Makefile

## Arquitectura backend
handler → service → repository. Sin SQL en handlers. Sin lógica de negocio en Gin.
Todo endpoint y query filtra por `empresa_id` del contexto (middleware de tenant).

## Reglas de trabajo
- NO inventes reglas de negocio. Si un requisito falta, DETENTE y pregunta (ver docs/).
- No cambies código adyacente ni reformatees lo que no toca la tarea.
- Toda función/endpoint nuevo va con test. Nada se marca "terminado" sin pruebas verdes.
- Nombres de dominio en español según el glosario (skill `gpvdp-domain`): Concepto, Clasificación, Movimiento, Cuadre, Traslado.

## Guardarraíl legal (OBLIGATORIO)
Nómina: calcular la base contributiva CCSS conforme a la ley (comisiones y bonificaciones habituales SON salario).
PROHIBIDO construir funciones o reportes cuyo propósito sea cuantificar u optimizar el "ahorro por no reportar"
salario a la CCSS (subdeclaración). Sí se permiten conceptos legítimamente no salariales con su base legal.
Ver skill `cr-fiscal-compliance`.

## Nunca
- Commitear `.env` ni credenciales. Borrado físico en tablas financieras. Datos de una empresa accesibles desde otra.
```

---

## 5. Estructura del monorepo

```
gpvdp-erp/
├── CLAUDE.md
├── Makefile
├── docker-compose.yml
├── .claude/
│   ├── agents/        # subagentes (§3)
│   ├── skills/        # skills nuevas (§2)
│   └── settings.json  # hooks
├── docs/              # spec funcional + contratos (OpenAPI, diccionario)
├── backend/
│   ├── cmd/api/
│   ├── internal/{bancos,catalogo,auth,tenant,shared}/   # por dominio
│   ├── migrations/
│   └── openapi.yaml
├── frontend/
│   ├── src/{features,components,lib,api}/   # api/ = cliente generado del OpenAPI
│   └── ...
└── infra/seed/        # datos de prueba
```

---

## 6. Stack tecnológico (versión-locked)

- **Backend:** Go 1.26, Gin, `pgx/v5` (+ `sqlc` para queries tipadas), `golang-migrate`, `go-playground/validator`, `zap` (logs), JWT con refresh, `excelize` (leer estados de cuenta), `testcontainers-go`.
- **Frontend:** Vite, React 18, TypeScript `strict`, Tailwind, **shadcn/ui** (Radix), **TanStack** Query/Table/Virtual, **Tremor** + Recharts (dashboards), **ECharts** (calendario de ingresos/gastos), `openapi-typescript` (cliente tipado), Vitest + RTL, Playwright.
- **DB:** PostgreSQL 16 (particionamiento por `(empresa_id, anio, mes)` en tablas calientes).
- **Local:** Docker Compose + Makefile + MailHog (para el buzón XML de CxP en Fase 2).

---

## 7. Entorno local para tus pruebas

**`docker-compose.yml`**
```yaml
services:
  db:
    image: postgres:16
    environment:
      POSTGRES_DB: gpvdp
      POSTGRES_USER: gpvdp
      POSTGRES_PASSWORD: localdev
    ports: ["5432:5432"]
    volumes: ["pgdata:/var/lib/postgresql/data"]
  backend:
    build: ./backend
    environment:
      DATABASE_URL: postgres://gpvdp:localdev@db:5432/gpvdp?sslmode=disable
    ports: ["8080:8080"]
    depends_on: [db]
  frontend:
    build: ./frontend
    environment:
      VITE_API_URL: http://localhost:8080
    ports: ["5173:5173"]
    depends_on: [backend]
  adminer:
    image: adminer
    ports: ["8081:8080"]
  mailhog:
    image: mailhog/mailhog
    ports: ["8025:8025"]
volumes: { pgdata: {} }
```

**`Makefile`**
```makefile
up:        ## Levanta todo el stack local
	docker compose up --build
migrate:   ## Aplica migraciones
	docker compose run --rm backend migrate up
seed:      ## Carga datos de prueba (3 empresas + catálogos + movimientos demo)
	docker compose run --rm backend go run ./cmd/seed
test:      ## Corre toda la batería de pruebas
	cd backend && go test ./... && cd ../frontend && npm test
e2e:       ## Pruebas end-to-end
	cd frontend && npx playwright test
```

Con esto pruebas en local: `make up` → API en `:8080`, UI en `:5173`, DB en Adminer `:8081`, correos en MailHog `:8025`.

---

## 8. Contratos (para que Claude Code no interprete)

**Diccionario de datos — núcleo de Bancos** (tipos PostgreSQL; Claude Code genera migraciones a partir de esto):

```
empresa(id uuid pk, nombre text, tipo_legal text, activo bool, creado_en timestamptz)

banco(id uuid pk, empresa_id uuid fk, nombre text, activo bool)
cuenta_bancaria(id uuid pk, empresa_id uuid fk, banco_id uuid fk, iban text, moneda text check in('CRC','USD'), alias text)
  unique(empresa_id, iban)

tipo_cambio_cotizacion(id uuid pk, empresa_id uuid fk, fecha date, valor numeric(14,4), fuente text check in('BCCR','MANUAL'))
tipo_cambio_mes(id uuid pk, empresa_id uuid fk, anio int, mes int, valor_congelado numeric(14,4) null,
                estado text check in('PROVISIONAL','CONGELADO'))
  unique(empresa_id, anio, mes)

importacion(id uuid pk, empresa_id uuid fk, cuenta_bancaria_id uuid fk, source_file_hash text, nombre_archivo text,
            estado text check in('CARGADA','PREVISUALIZADA','CONFIRMADA','CERRADA'), creado_por uuid, creado_en timestamptz)

movimiento_bancario(
  id uuid pk, empresa_id uuid fk, cuenta_bancaria_id uuid fk, importacion_id uuid fk,
  fecha date, documento text, descripcion text,
  debito numeric(16,2) default 0, credito numeric(16,2) default 0,
  moneda_original text, monto_original numeric(16,2), monto_crc numeric(16,2), tc_aplicado numeric(14,4) null,
  concepto_id uuid fk null, clasificacion_id uuid fk null,
  estado_clasificacion text check in('NO_IDENTIFICADO','AUTO','REVISADO'), confianza numeric(5,2) null,
  es_traslado bool default false, par_traslado_id uuid null,
  natural_key text, indice_ocurrencia int default 1, incluido bool default true, origen_historico bool default false,
  creado_en timestamptz, actualizado_en timestamptz)
  index(empresa_id, fecha) · index(empresa_id, estado_clasificacion) · unique(empresa_id, natural_key)

concepto(id uuid pk, empresa_id uuid fk, nombre text, activo bool)
clasificacion(id uuid pk, empresa_id uuid fk, concepto_id uuid fk, nombre text, cuenta_contable_futura text null, activo bool)
regla_clasificacion(id uuid pk, empresa_id uuid fk, nombre text, aplica_a text check in('DEBITO','CREDITO','MIXTO'),
                    concepto_id uuid fk, clasificacion_id uuid fk, prioridad int, activo bool)
palabra_clave(id uuid pk, regla_id uuid fk, texto text)

auditoria_evento(id uuid pk, empresa_id uuid fk, entidad text, entidad_id uuid, accion text,
                 valor_anterior jsonb, valor_nuevo jsonb, usuario_id uuid, ts timestamptz)  -- append-only
```

**OpenAPI 3.1** — generar `backend/openapi.yaml` a partir de los endpoints del §24 de la spec funcional, con esquemas de request/response por endpoint. Es el contrato del que el frontend deriva su cliente tipado. (Artefacto a producir en Fase 0; te lo entrego cuando confirmes el diccionario.)

---

## 9. Diseño premium (más allá de Hope UI)

Hope UI es una plantilla Bootstrap/Chakra; "premium" aquí significa **sistema propio, no plantilla**. Dirección (apóyate en tu skill `ui-ux-pro-max`):

- **Base:** Tailwind + shadcn/ui (control total, sin *look* de plantilla). Evitar el patrón "IA genérica" (Inter + gradiente morado sobre blanco).
- **Datos:** TanStack Table + Virtual para Movimientos/Revisar/Cuadre (miles de filas, filtros densos, virtualización). Encabezado con totales vivos (débitos/créditos/diferencia).
- **Dashboards:** Tremor + Recharts para KPIs/EBITDA/comparativos; **ECharts** para los calendarios de ingresos y gastos (heatmap por día). Modo claro/oscuro.
- **Tokens definidos:** paleta financiera sobria (neutros + un acento), tipografía con jerarquía (números tabulares para montos), grilla de 8px, estados de tabla (positivo/negativo/pendiente). Densidad "cómoda" configurable a "compacta" para la hoja de trabajo.
- **Accesibilidad:** foco visible, contraste AA, navegación por teclado en tablas.

---

## 10. Estrategia de pruebas + Definition of Done

**Pruebas (todo se testea):**
- Backend: tests de tabla por service + integración con `testcontainers-postgres` (importador, congelamiento de TC, dedup, motor de clasificación, cuadre). Cobertura mínima **80%** en dominio financiero.
- Frontend: Vitest + RTL por componente; **Playwright E2E** de los flujos críticos.
- CI: `make test` + `make e2e` en cada PR; lint (`golangci-lint`, `eslint`) y `gofmt` por hook.

**Definition of Done (un módulo/entregable está "terminado" solo si):**
1. Cumple los casos de uso y reglas de la spec funcional. 2. Pruebas verdes y cobertura mínima. 3. Aislamiento por empresa verificado. 4. Auditoría registrando los eventos. 5. Revisado por `security-reviewer`. 6. Corre en `make up` y es probable en tu local. 7. Sin TODOs de negocio sin resolver.

---

## 11. Plan por fases

- **Fase 0 — Cimientos (1 sprint):** monorepo, docker/Makefile, migraciones base, auth + JWT, middleware multi-tenant, RBAC, selector de empresa, seed de 3 empresas. `CLAUDE.md`, agentes y skills nuevas creadas. **Entregable probable en local.**
- **Fase 1 — Bancos completo:** Importador (+ adaptadores por banco [T1]), Catálogo + motor de clasificación (aprendizaje, diccionario portable), motor de TC (congelamiento), dedup, Revisar, Movimientos, Cuadre, Exportar (incl. consecutivo largo Davivienda), Dashboard, Proyecciones. Emparejamiento de traslados.
- **Fase 2 — CxP:** buzón XML (MailHog en local), maestro de proveedores, flujo de 6 estados, aprobaciones (1M/5M/GG + mancomunada), archivo bancario/respuesta, huella Bancos↔CxP, reportería.
- **Fase 3 — Nómina (compliant):** conceptos parametrizables, base CCSS conforme a ley, deducciones por tags, tramos de renta, ciclo quincenal, SINPE, boleta, costo real/provisiones — **sin** la función de "ahorro por no reportar".
- **Fase 4 — Presupuesto + OC:** presupuesto configurable (vs real, alertas, KPIs) alimentado por CxP; módulo OC.
- **Luego:** ajustar con datos reales (formatos, EBITDA, roles).

---

## 12. Insumos de negocio que aún faltan (bloquean el 100%)

- **[T1]** Un Excel de muestra por banco (BCR, BAC, BN, Davivienda…) → adaptadores del importador.
- **[T2]** Fórmula/perímetro de EBITDA (qué conceptos son operativos).
- **[T3]** Matriz fina rol × acción × empresa.
- **Decisión A:** tolerancia del diferencial en traslados USD (monto fijo o %).
- **Decisión B:** cierre de período bloqueante (no avanza con "No identificado") o solo advertencia.

Claude Code puede construir **Fase 0 completa y el 90% de Fase 1** sin estos; los adaptadores de banco quedan como *stubs* hasta que llegue [T1].

---

## Siguiente paso sugerido

1. Confirmas el **diccionario de datos (§8)** y las **decisiones A/B**.
2. Con eso te genero el **OpenAPI 3.1 de Bancos** y las **skills nuevas** (`go-gin-backend`, `gpvdp-domain`, `cr-fiscal-compliance`) listas para pegar en `.claude/skills/`.
3. Arrancás Fase 0 en Claude Code con este kit.

*Fin del Build Kit v1.0. Documentación, no implementación: Claude Code escribe el código; este kit evita que lo escriba genérico.*
