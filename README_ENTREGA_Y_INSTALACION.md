# GPVDP ERP — Entrega para Claude Code (LÉEME PRIMERO)

Este bundle es todo lo que necesitas para que Claude Code construya el ERP **sin salir genérico**.
Sigue el orden: primero **instalas**, luego **copias los archivos al repo**, luego **arrancas Fase 0**.

---

## 1. Qué instalar ANTES de pasarle los .md

### 1.1 Toolchain local (para poder probar en tu máquina)
- **Docker + Docker Compose** (corre todo el stack local).
- **Go 1.26**, **Node 24 LTS**, **Git**.
- Claude Code instala el resto (golang-migrate, sqlc, golangci-lint, openapi-typescript, Playwright) durante Fase 0; no necesitas hacerlo a mano.

### 1.2 Skill base de Go (OPCIONAL — no en la ruta crítica)
La skill de proyecto `go-gin-backend` ya es AUTOSUFICIENTE (incluye Go moderno + convenciones GPVDP),
así que NO necesitás instalar nada externo para arrancar. Si querés la capa extra de idiomatica Go,
podés (opcional) instalar `cc-skills-golang`; si no carga en tu entorno de Code, ignoralo: no bloquea nada.
> Nota: en la app de escritorio, las skills locales/plugin no aparecen en "Personalizar → Skills"
> (solo se ven las `anthropic-skills:`), pero funcionan igual. No es un error.

### 1.3 (Opcional) Generador de skills
```
/plugin install skill-creator@anthropic-agent-skills
```

### 1.4 Tus skills existentes — cuáles dejar activas
**Deja ACTIVAS** (aportan a este proyecto): `ui-ux-pro-max`, `tdd-workflow`, `security-audit`,
`api-architect`, `database-architect`, `system-design`, `frontend-testing`, `fintech-engineer`,
`generating-reports`, `hook-development`, `coding-agent`, `skill-creator`, `analisis-horizontal-financiero`,
`backend-code-review`, `backend-patterns`.

**DESACTIVA durante el build** (para evitar disparos ruidosos y salidas genéricas):
`agent-whatsapp`, `lead-intelligence`, `market-research`, `carrier-relationship-management`,
`content-creator`, `frontend-slides`, `efficient-inversion-count-calculation-in-python`,
`frontend-query-mutation` (es oRPC/Dify; aquí lo reemplaza `gpvdp-data-layer`).

---

## 2. Qué contiene este bundle

```
GPVDP_ENTREGA/
├── README_ENTREGA_Y_INSTALACION.md   <- este archivo
├── CLAUDE.md                         <- memoria de proyecto (reglas siempre activas)
├── docker-compose.yml                <- stack local
├── Makefile                          <- up / migrate / seed / test / e2e
├── .claude/
│   ├── settings.json                 <- hooks (gofmt, bloqueo de .env)
│   ├── agents/                       <- 5 subagentes (backend, frontend, db, qa, security)
│   └── skills/                       <- 6 skills de proyecto
│       ├── go-gin-backend/           <- convenciones Go+Gin de GPVDP (anti-genérico backend)
│       ├── gpvdp-domain/             <- glosario + reglas de negocio (núcleo anti-genérico)
│       ├── cr-fiscal-compliance/     <- fiscal/laboral CR + guardarraíl de nómina
│       ├── multi-tenant-postgres/    <- aislamiento por empresa
│       ├── bank-import-adapters/     <- adaptadores de banco (stubs hasta [T1])
│       └── gpvdp-data-layer/         <- capa de datos frontend (OpenAPI + TanStack Query)
└── docs/
    ├── GPVDP_Modulo_Bancos_EspecFuncional_v1.0.md
    ├── GPVDP_BuildKit_ClaudeCode_v1.0.md
    └── openapi-bancos.yaml           <- contrato del que el frontend genera su cliente
```

---

## 3. Cómo pasarlo a Claude Code
1. Crea el repo `gpvdp-erp/` y **copia el contenido de este bundle en la raíz** (así `.claude/`, `docs/`, `CLAUDE.md` quedan donde Claude Code los lee).
2. Abre Claude Code en esa carpeta. Corre `/agents` y `/skills` para confirmar que ve los 5 subagentes y las 6 skills de proyecto + la base de Go.
3. Si creaste las carpetas con la sesión abierta, **reinicia Claude Code** (los directorios nuevos solo se detectan al iniciar).

---

## 4. SETEA tus 2 decisiones antes de arrancar
En `CLAUDE.md` y en la skill `gpvdp-domain` están estos parámetros como pendientes:
- **TOLERANCIA_TRASLADO_USD**: monto fijo en CRC para emparejar traslados USD con diferencial. (Diste "monto fijo"; falta el número, ej. ₡500.)
- **CIERRE_PERIODO_BLOQUEANTE**: `true` (no cierra con No-identificados) o `false` (solo advierte).

Reemplaza los "(SETEAR)" con tus valores. Si no los pones, el agente se detendrá y te preguntará (por diseño).

---

## 5. Prompt de arranque — Fase 0 (copia y pega en Claude Code)

```
Lee CLAUDE.md, docs/GPVDP_BuildKit_ClaudeCode_v1.0.md y docs/GPVDP_Modulo_Bancos_EspecFuncional_v1.0.md.
Vamos a construir la Fase 0 del ERP GPVDP. NO implementes lógica de negocio de Bancos todavía.

Alcance Fase 0 (entregable probable en local con `make up`):
1) Estructura del monorepo (backend Go+Gin, frontend Vite+React+TS) según el Build Kit.
2) docker-compose + Makefile funcionando (db, backend, frontend, adminer, mailhog).
3) Migraciones base + tablas del diccionario de datos (empresa, usuario, rol, banco, cuenta_bancaria,
   catálogos, movimiento_bancario, tipo_cambio_*, importacion, auditoria_evento).
4) Auth con JWT (+refresh), middleware multi-tenant (empresa_id en contexto), RBAC básico.
5) Selector de empresa en el frontend. Seed de 3 empresas (Valle de Paz, Coopeprofa, Memorial Pets).
6) Cliente frontend generado desde docs/openapi-bancos.yaml.

Usa el subagente backend-go para el backend y db-postgres para el esquema. Aplica las skills
go-gin-backend, gpvdp-domain y multi-tenant-postgres. Al terminar, el subagente qa-tdd escribe pruebas
y verifica el Definition of Done, y security-reviewer revisa el aislamiento por empresa.
No inventes reglas: si falta un dato, detente y pregúntame. Trabaja por PRs pequeños y testeados.
```

Cuando Fase 0 pase el Definition of Done y corra en tu local, seguimos con **Fase 1 — Bancos completo**.

---

## 6. Checklist final antes de arrancar
- [ ] Docker/Go/Node instalados.
- [ ] (Opcional) `cc-skills-golang` instalada. Si no carga, se ignora: `go-gin-backend` es autosuficiente.
- [ ] Skills ruidosas desactivadas.
- [ ] Bundle copiado a la raíz del repo.
- [ ] `/agents` y `/skills` muestran todo.
- [ ] TOLERANCIA_TRASLADO_USD y CIERRE_PERIODO_BLOQUEANTE seteados (o listo para que el agente pregunte).
- [ ] Pendiente de negocio: [T1] muestras de banco, [T2] fórmula EBITDA, [T3] matriz de roles (no bloquean Fase 0).
```
```
