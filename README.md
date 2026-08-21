# GPVDP ERP — Monorepo

ERP financiero multiempresa (Valle de Paz · Coopeprofa · Memorial Pets · escalable).
Multi-tenant con aislamiento por empresa. Este repo contiene el **backend Go+Gin**, el
**frontend Vite+React+TS** y la infraestructura local (Docker Compose).

> Documentos de referencia: [`CLAUDE.md`](CLAUDE.md) (reglas siempre activas),
> [`README_ENTREGA_Y_INSTALACION.md`](README_ENTREGA_Y_INSTALACION.md) (entrega),
> y [`docs/`](docs/) (especificación funcional de Bancos + OpenAPI).

---

## Estado

**Fase 0 — Cimientos ✅**  ·  **Fase 1 — Bancos: backend avanzado ✅ / frontend ⏳**

### Fase 0 (cimientos)
- **Monorepo**: `backend/` (Go 1.26 + Gin, capas handler→service→repository) y `frontend/` (Vite + React + TS estricto).
- **Infra**: `docker-compose.yml` + `Makefile` (db, backend, frontend, adminer, mailhog).
- **DB**: migraciones `golang-migrate` embebidas y **auto-aplicadas al arrancar** (tablas del diccionario;
  auditoría append-only reforzada con trigger anti-TRUNCATE; FK compuesta concepto↔clasificación).
- **Auth**: login **JWT + refresh** rotativo/revocable (hash en DB), bcrypt; re-valida usuario activo en refresh y selección de empresa.
- **Multi-tenant**: la **empresa activa viaja dentro del access token** (claim `empresa_id`), nunca por body/query.
  Middleware `RequireAuth` / `RequireEmpresa` / `RequireRol`.
- **Seed idempotente**: 3 empresas + 6 roles + admin + catálogo demo + **13 cuentas reales de Valle de Paz**.
- **Frontend**: login, **selector de empresa**, shell autenticado, cliente HTTP tipado (generado del OpenAPI).

### Fase 1 — Bancos (backend)
- **Importador**: parseo de los **7 formatos** (Promerica, BN, BAC, BCR, Banco Popular, Davivienda), detección por firma,
  **dedup** (nuevo / duplicado real / reimportación), memoria IBAN, guarda el archivo original. Verificado contra las 13 muestras reales.
- **Clasificación**: motor por reglas + palabras clave (**auto ≥90%** al importar, no pisa lo revisado),
  reclasificación manual, y **crear regla** que aprende sobre el bloque no identificado.
- **Tipo de cambio**: cotizaciones día 1/15/último, **provisional escalonado**, **congelamiento** retroactivo a los USD del mes
  (inmutable), protegido por rol.
- **Cuadre** por concepto y **Dashboard** (Ingresos / Gastos / **EBITDA = Ingresos − Gastos** + comparativo mes anterior).
- **Traslados/Overnight**: emparejamiento débito↔crédito entre cuentas del grupo con propuestas automáticas (**tolerancia 1%**); los pares emparejados se excluyen del EBITDA.
- **Cierre de período bloqueante**: no cierra con movimientos "No identificado" pendientes (configurable, `CIERRE_PERIODO_BLOQUEANTE`).
- **Pruebas backend**: `go test ./...` verde (parseo real, dedup, clasificación, matemática de TC y tolerancia, JWT, aislamiento/RBAC). **Revisión adversarial aplicada** (backend y frontend).

### Fase 1 — Bancos (frontend)
- **7 pantallas** (TanStack Query, cliente tipado del OpenAPI): **Dashboard** (KPIs + EBITDA + comparativo + cuadre + cerrar período), **Importador** (subir Excel → preview con chips de duplicado → confirmar), **Movimientos** (filtros del servidor + totales vivos + paginación), **Revisar** (clasificar selección + crear regla por bloque), **Traslados** (emparejar/desemparejar), **Catálogo** y **Tipo de cambio** (cotizaciones + congelar).

> **Fase 1 completa.** Falta solo la corrida real con `make up` para la verificación end-to-end (tsc + build + migraciones + prueba subiendo los Excel).

---

## Requisitos

- **Docker + Docker Compose** (levanta todo el stack).
- Opcional para desarrollo directo: **Go 1.26** (backend) y **Node 24** (frontend). No hacen falta si usás Docker.

## Arranque rápido

```bash
make up        # construye y levanta todo; el backend migra y siembra solo
```

Servicios:

| Servicio | URL | Notas |
|---|---|---|
| Frontend | http://localhost:5173 | Vite dev server |
| Backend API | http://localhost:8080/v1 | health: `/v1/healthz` |
| Adminer (DB) | http://localhost:8081 | solo desde esta máquina; credenciales en `docker-compose.yml` |
| MailHog | http://localhost:8025 | buzón de correo (para CxP en Fase 2) · solo desde esta máquina |

Con la prueba en la oficina, el frontend y la API también responden en `http://192.168.1.115:5173`
y `:8080`. Adminer, MailHog y la base **no**: están atados a esta máquina a propósito.

### Credenciales

- El usuario administrador es `admin@gpvdp.local`. La contraseña **no se documenta acá**: se cambia
  desde la pantalla de cambio de contraseña al primer ingreso y queda guardada en la base.
- El admin pertenece a las 3 empresas → al entrar verás el **selector de empresa**.
- Cada persona usa su propia cuenta, creada en **Configuración → Usuarios**. No se comparte el admin.

## Comandos útiles

```bash
make help          # lista todos los targets
make down          # apaga el stack (conserva datos)
make logs          # logs del backend
make migrate       # aplica migraciones (el backend ya lo hace al arrancar)
make seed          # re-siembra datos base
make test          # pruebas del backend (frontend requiere Node)
```

## Probar el módulo Bancos por API

Mientras no estén las pantallas, el flujo se prueba por API (Postman/curl). Con `make up` corriendo:

1. **Login** → `POST /v1/auth/login` `{ "email":"admin@gpvdp.local", "password":"<tu contraseña>" }` → guardá `access_token` y el `id` de **Valle de Paz** (viene en `empresas`).
2. **Elegir empresa** → `POST /v1/auth/select-empresa` `{ "empresa_id":"<id>" }` con `Authorization: Bearer <access_token>` → **nuevo** `access_token` (ya con empresa). Todo lo de abajo usa este token.
3. **Cuentas** → `GET /v1/bancos/cuentas` → copiá el `id` de la cuenta según el archivo (ej. `BAC Religiosa`).
4. **Importar** → `POST /v1/bancos/importaciones` (multipart: `cuenta_bancaria_id` + `archivo` = `.xlsx`) → **preview** con resumen y líneas marcadas `NUEVO` / `DUPLICADO_REAL` / `REIMPORTACION`.
5. **Confirmar** → `POST /v1/bancos/importaciones/{id}/confirmar` `{ "excluir": [] }` → `{ insertados }`. Reimportar el mismo archivo → `insertados: 0` (idempotente).
6. **Ver / clasificar** → `GET /v1/bancos/movimientos?estado_clasificacion=NO_IDENTIFICADO` · `POST /v1/bancos/reglas` (aprende) · `PATCH /v1/bancos/movimientos/{id}/clasificacion`. Catálogo: `GET /v1/bancos/catalogo/conceptos|clasificaciones`.
7. **Tipo de cambio** → `POST /v1/bancos/cotizaciones` (día 1/15/último) · `GET /v1/bancos/tipo-cambio/{anio}/{mes}` · `POST .../congelar`.
8. **Reportes** → `GET /v1/bancos/cuadre?periodo=2026-06` · `GET /v1/bancos/dashboard?periodo=2026-06`.

## Variables de entorno

Ver [`.env.example`](.env.example). Para desarrollo con `make up` los valores ya vienen inline en
`docker-compose.yml`. **Nunca** commitear `.env` ni credenciales reales.

---

## Decisiones de negocio (resueltas)

- **EBITDA = Ingresos − Gastos** (traslados/overnight emparejados excluidos).
- **TOLERANCIA_TRASLADO = 1%** (diferencia máxima entre las patas de un traslado/overnight).
- **CIERRE_PERIODO_BLOQUEANTE = true** (no cierra el período con movimientos "No identificado" pendientes).
- **Roles**: los 6 base + permisos agregables/seleccionables (matriz configurable).
- **[T1] muestras de banco**: recibidas y documentadas en [`docs/GPVDP_Formatos_Bancos_v1.0.md`](docs/GPVDP_Formatos_Bancos_v1.0.md).
  Todas las cuentas pertenecen a **Valle de Paz**; Davivienda = 1 banco con 3 cuentas.

## Pendiente

- **Verificación end-to-end**: correr `make up` y probar subiendo los Excel reales (Docker compila tsc + Go + aplica migraciones).
- **Detalles de frontend**: crear conceptos/clasificaciones (falta `POST` en el contrato) y listar traslados ya emparejados (falta `GET`); Exportar / Proyecciones / Conciliación quedan para después.
- **[T3]** matriz fina rol × acción × empresa (el mecanismo RBAC ya existe; la matriz se configura).
- **Fase 2+**: CxP (buzón XML con MailHog), Nómina (compliant), Presupuesto/OC.

## Notas técnicas

- `movimiento_bancario` se deja como tabla simple con índices compuestos; **particionar por `(empresa_id, anio, mes)`
  queda para Fase 1** cuando entre el volumen real.
- Los subagentes y skills del proyecto viven en `.claude/`. Para que Claude Code los vea como propios,
  abrí Claude Code **en esta carpeta** (raíz del repo) y reiniciá.
- **Endurecimiento pendiente (Fase 1):** crear un rol de PostgreSQL de mínimo privilegio para la app
  (distinto del dueño de las tablas) y `REVOKE TRUNCATE/UPDATE/DELETE ON auditoria_evento`, para reforzar
  la inmutabilidad más allá de las rules + el trigger anti-TRUNCATE ya incluidos.
