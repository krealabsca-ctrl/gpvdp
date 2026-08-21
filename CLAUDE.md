# GPVDP ERP — Memoria de Proyecto

## Qué es
ERP financiero multiempresa (Valle de Paz, Coopeprofa, Memorial Pets · escalable).
Multi-tenant con aislamiento por empresa. Este ERP es el sistema de registro (no hay contabilidad externa).
Un usuario puede operar una o varias empresas.

## Stack (NO desviarse sin aprobación)
- Backend: Go 1.26 + Gin · pgx/sqlc · golang-migrate · go-playground/validator · JWT(+refresh) · zap
- Frontend: Vite + React + TypeScript estricto · Tailwind + shadcn/ui · TanStack Query/Table/Virtual · Tremor/Recharts/ECharts
- DB: PostgreSQL 16
- Infra local: Docker Compose · Makefile · MailHog (CxP)

## Arquitectura backend
handler (Gin) -> service (negocio) -> repository (datos). Sin SQL en handlers. Sin lógica de negocio en Gin.
TODO endpoint y query filtra por empresa_id del contexto (middleware de tenant). Ver skill go-gin-backend.

## Reglas de trabajo
- NO inventes reglas de negocio. Si un requisito falta, DETENTE y pregunta (ver docs/).
- No cambies ni reformatees código adyacente que la tarea no pide.
- Toda función/endpoint nuevo va con test. Nada se marca "terminado" sin pruebas verdes (ver Definition of Done).
- Nombres de dominio en español según glosario (skill gpvdp-domain): Empresa, MovimientoBancario, Concepto, Clasificacion, Cuadre, Traslado.
- Dinero: nunca float64. numeric en DB -> decimal en Go.

## Guardarraíl legal (OBLIGATORIO)
Nómina: calcular la base contributiva CCSS conforme a la ley (comisiones y bonificaciones habituales SON salario).
PROHIBIDO construir funciones o reportes cuyo propósito sea cuantificar u optimizar el "ahorro por no reportar"
salario a la CCSS (subdeclaración). Sí se permiten conceptos legítimamente no salariales con su base legal.
Ver skill cr-fiscal-compliance.

## Parámetros de negocio (decisiones del Director Financiero)
- EBITDA = Ingresos − Gastos (los traslados/overnight emparejados NO cuentan). CONFIRMADO.
- RBAC: usar los roles base (ADMIN, DIRECTOR_FINANCIERO, SUPERVISOR_FINANCIERO, AUXILIAR_FINANCIERO,
  GERENCIA_GENERAL, AUDITOR_INTERNO) y permitir crear roles y asignar permisos de forma SELECCIONABLE
  (matriz permiso×rol×empresa configurable en UI). CONFIRMADO.
- Empresas/cuentas: TODAS las cuentas de las muestras pertenecen a la empresa "Valle de Paz" (aunque el
  titular diga Jardines, Colinas, Religiosa, Privado de Cartago, COPENAE). Davivienda = 1 banco con 3 cuentas
  separadas (Colones, Comisiones COPENAE, Dólares). Ver docs/GPVDP_Formatos_Bancos_v1.0.md.
- TOLERANCIA_TRASLADO = 1% (emparejar traslado/overnight si las dos patas difieren ≤ 1% del monto;
  porcentaje, no monto fijo; aplica sobre todo a USD por el diferencial cambiario). CONFIRMADO.
- CIERRE_PERIODO_BLOQUEANTE = true (NO se puede cerrar el período con movimientos "No identificado"
  pendientes; hay que clasificar el 100% antes de cerrar). CONFIRMADO.

## Formatos de banco (Fase 1)
7 layouts en docs/GPVDP_Formatos_Bancos_v1.0.md: Promerica, BN (5 cuentas, sin IBAN), BAC (dup. intra-archivo),
BCR (IBAN CC-, "-"=0), BP (montos con prefijo CRC, fecha "01 JUN 2026"), Davivienda (3 cuentas, overnight).

## Nunca
- Commitear .env ni credenciales. Borrado físico en tablas financieras. Datos de una empresa accesibles desde otra.
