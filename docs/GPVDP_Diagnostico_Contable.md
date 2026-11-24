# ¿Soporta este ERP una contabilidad formal? Diagnóstico de estructura y viabilidad

**23 de octubre de 2026** · Sistema GPVDP, migración 108 · **Diagnóstico, no desarrollo.**

Todo número está medido contra la base real. La pregunta que contesta este documento es la que
planteó el Director Financiero: **si la estructura lo soporta, qué tan complicado es implementarlo
y qué tan viable es según los criterios del mandato.**

---

## 0 · Corrección de la versión anterior

La primera versión de este diagnóstico tenía tres errores de lectura. Se corrigen acá porque
cambian conclusiones:

1. **No hay «un catálogo de 398 clasificaciones».** Hay **tres catálogos independientes, uno por
   empresa**. Tratarlos como uno solo subestima el trabajo de mapeo y oculta que hoy no existe un
   plan común del grupo.

2. **`CATALOGO DF.1.xlsx` no es «el catálogo contable» a secas.** Es un **trabajo de
   reestructuración en curso**: compara el catálogo que se propone (columna ACTUAL) contra el que
   hoy vive en **Softland**. Eso cambia el punto de partida: el catálogo no hay que diseñarlo,
   hay que terminar de migrarlo.

3. **Los dos Excel no viven dentro del ERP.** Son archivos externos. El ERP tiene su propio
   catálogo de preclasificación, que es otra cosa y con otro propósito.

---

## 1 · Las tres fuentes, y qué es cada una

| Fuente | Qué es | Tamaño | Propósito |
|---|---|---:|---|
| **Softland** | Catálogo contable vigente | 258 cuentas, 5 niveles | Contabilidad formal e histórico |
| **`CATALOGO DF.1.xlsx`** | Reestructuración propuesta | 198 iguales · 30 reubicadas · 30 nuevas · 5 que salen | Migrar el plan de cuentas |
| **ERP GPVDP** | Preclasificación por empresa | VDP 21/170 · Memorial Pets 28/165 · Coopeprofa 15/63 | Análisis gerencial y control de gasto |

El `diccionario-catalogo (12).xlsx` es la exportación del catálogo del **ERP para Valle de Paz**:
20 conceptos y 220 clasificaciones, donde el concepto *Gastos* concentra **155**.

### Lo que esto significa

**Son dos planos distintos, y los dos tienen razón de existir.** El catálogo contable responde
«¿en qué cuenta se registra?»; la preclasificación responde «¿en qué se nos va la plata?». La
segunda es más fina a propósito: *ACEITE*, *Almuerzos Jason*, *Alquiler Equipo de Limpieza* son
detalle gerencial que ninguna contabilidad lleva como cuenta.

**No hay que fusionarlos ni reemplazar uno por otro. Hay que relacionarlos**: varias
clasificaciones caen en la misma cuenta contable, y eso es correcto.

---

## 2 · ¿La estructura lo soporta?

**Sí.** Y más de lo que esperaba, por siete razones concretas y verificadas:

### 2.1 · El gancho ya existe y está vacío

La tabla `clasificacion` tiene una columna **`cuenta_contable_futura`** desde el diseño original.
El backend la acepta al crear una clasificación, el frontend la expone, y **está vacía en las 398
filas**. Alguien previó este día y dejó el enganche puesto.

### 2.2 · El dinero nunca pasó por punto flotante

`numeric(16,2)` en la base → `decimal` en Go → **string** en JSON. Nunca `float64`. Esto no es un
detalle: **un sistema contable que guarda dinero en coma flotante no es auditable**, y rehacerlo
después es reescribirlo entero. Es el requisito más caro de corregir tarde y acá está resuelto.

### 2.3 · La factura de CxP ya trae las patas del asiento

`documento_cxp` tiene: `subtotal`, `iva`, `retencion`, `total`, `total_crc`, `moneda`,
`proveedor_id`, `fecha_emision`, `clasificacion_id`, `departamento_id` y la clave de Hacienda.

Un asiento de compra necesita exactamente eso. **Los datos no hay que inventarlos ni volver a
capturarlos: ya se están capturando.**

### 2.4 · Multiempresa real, no un filtro cosmético

`empresa_id` en todas las tablas, verificado contra fugas con pruebas que fallan si se rompe. La
contabilidad multiempresa y la consolidación se apoyan en esto, y es lo más difícil de agregar
después.

### 2.5 · Las dimensiones ya están

`departamento_id` y `sede_id` cuelgan de la clasificación, ortogonales a la partida. Centro de
costo y unidad de negocio no hay que diseñarlos.

### 2.6 · Trazabilidad hasta el papel

La importación guarda **el archivo del banco entero**. CxP guarda la factura y el comprobante.
`auditoria_evento` es append-only. El requisito de «rastrear el asiento hasta su documento de
origen» tiene de dónde agarrarse.

### 2.7 · Arquitectura que admite un módulo nuevo sin tocar los viejos

`handler → service → repository`, migraciones reversibles embebidas, puertos entre módulos (el de
Bancos↔CxP ya funciona así). **Un módulo de contabilidad se agrega al lado, no adentro.** Si se
apagara, el ERP seguiría igual.

### Lo que la estructura NO tiene

| Falta | Peso |
|---|---|
| Tabla de cuentas contables con jerarquía | Se crea; no hay nada que migrar |
| Asiento y línea de asiento con partida doble | **El corazón del trabajo** |
| Mayor, balance de comprobación, saldos iniciales | Derivados del asiento |
| Período *contable* (el que existe es de clasificación bancaria) | Distinto y adicional |
| Activos fijos y depreciación | Módulo completo que no existe |
| Plan de cuentas **común al grupo** | Hoy cada empresa tiene su catálogo propio |

---

## 3 · ¿Qué tan complicado es?

Medido en piezas concretas, no en tiempo. El orden es obligatorio: cada fase necesita la anterior.

| Fase | Qué se construye | Tamaño | Riesgo sobre lo existente |
|---|---|---|---|
| **1 · Plan de cuentas** | 1 migración, importador del Excel, 1 pantalla | **Bajo** · hay patrón (el catálogo de Bancos hace esto) | **Nulo**: tabla nueva que nadie lee |
| **2 · Mapeo** | Equivalencias clasificación → cuenta, pantalla, reporte de cobertura | **Bajo** · la columna ya existe | **Nulo** |
| **3 · Motor de asientos** | Asiento y línea, invariante de partida doble en la escritura, generadores por documento, diario y mayor | **Alto** · es el 60 % del trabajo | **Bajo**: solo lee los documentos |
| **4 · Apertura y cierre** | Saldos iniciales, balance de comprobación, período contable | **Medio** | **Bajo** |
| **5 · Estados financieros** | Situación, resultados, flujo de efectivo, cambios en patrimonio | **Medio-alto** · el flujo de efectivo es el difícil | **Nulo**: solo lee |
| **6 · Consolidación** | Eliminaciones entre compañías, partes relacionadas | **Medio** | **Nulo** |

### Por qué el riesgo sobre lo existente es bajo

Porque **la contabilidad no modifica nada: lee**. Los asientos se generan *desde* la factura, el
cargo y la planilla, y se escriben en tablas nuevas. Ninguna fase cambia cómo se importa un
estado de cuenta, cómo se aprueba una factura o cómo se paga.

La única pieza que toca algo existente es el mapeo, y lo hace llenando una columna que hoy está
vacía.

### Dónde está la dificultad real

No está en el código. Está en tres cosas:

1. **La partida doble hay que garantizarla en la escritura, no comprobarla después.** Un asiento
   desbalanceado no puede llegar a existir. Es una invariante de base de datos, y es la decisión
   técnica que define si el sistema es confiable o no.

2. **Devengo, no caja.** Es el punto que ya se marcó en la versión anterior y se sostiene: si los
   asientos se generan desde el movimiento bancario, se contabiliza cuando la plata se mueve, y
   eso no produce NIIF. Hay que generarlos desde el documento y usar el banco para cancelar y
   conciliar. **El sistema ya tiene ese patrón funcionando en Bancos↔CxP.**

3. **Los 220 mapeos de Valle de Paz** (más los de las otras dos empresas) **los decide un
   contador, no un programador.** Es trabajo humano y es el que marca el ritmo.

---

## 4 · ¿Qué tan viable, según los criterios del mandato?

| Criterio | Veredicto | Sustento |
|---|---|---|
| Catálogo maestro y equivalencias | **Alta** | Ya existe y está en migración; el gancho en el ERP está puesto |
| Automatizar asientos desde transacciones | **Alta** | CxP tiene IVA, retención, proveedor y clasificación; CxC tiene 70.592 cargos |
| Estados financieros conciliables | **Media** | Depende por completo de los saldos iniciales |
| Cierres mensuales | **Media** | El mecanismo existe y **nunca se ha ejercido**: ningún período cerrado hasta hoy |
| Rentabilidad, liquidez, capital de trabajo | **Alta** | Las dimensiones están; faltan las cuentas |
| EBITDA conciliado con el resultado contable | **Media** | El EBITDA sale de la naturaleza del concepto, y el sistema permite que un concepto no la tenga declarada |
| Consolidación y eliminaciones | **Media** | Multiempresa resuelto; las operaciones entre compañías hoy son una partida más, no una eliminación |
| Preparación para due diligence | **Baja hoy** | Sin cierres ejercidos, sin saldos iniciales y sin forma de separar las partes relacionadas |
| Control interno y trazabilidad | **Alta** | Auditoría, permisos y archivo original ya están |

### Los tres bloqueos reales

Ninguno es de programación:

**1 · Los saldos iniciales.** Sin apertura no hay balance; como máximo un estado de resultados del
período. Salen de Softland o de un balance certificado. **No se pueden inventar.**

**2 · El contador que valida.** El catálogo, las políticas de reconocimiento y el tratamiento
fiscal costarricense los define un profesional colegiado. Construir la Fase 1 sin él es construir
a ciegas.

**3 · El cierre de período que nunca se ha ejercido.** Mientras no se cierre, cualquier cifra
histórica puede cambiar retroactivamente. Para un comprador, eso es lo primero que falla en una
revisión, y el mecanismo ya está construido: falta usarlo.

---

## 5 · Sobre las cifras de negocio: no son concluyentes

La versión anterior de este diagnóstico sacaba conclusiones de los montos cargados en la base
—dinero sin clasificar, partidas con partes relacionadas—. **Esas conclusiones quedan retiradas:
los datos cargados son de prueba**, según confirmó el Director Financiero.

Lo que **sí** se sostiene de esas mediciones es lo **estructural**, porque no depende de que los
montos sean reales:

- **El sistema permite que un movimiento quede sin clasificar**, y nada obliga a resolverlo antes
  de cerrar salvo el cierre de período, que existe y no se ha usado.
- **La naturaleza (ingreso/gasto/neutro) se declara por concepto y puede no declararse.** Mientras
  haya conceptos sin declarar, el EBITDA se calcula sobre un supuesto, no sobre una afirmación.
- **Las operaciones entre empresas del grupo se registran como una partida más** del catálogo de
  cada empresa, no como una cuenta entre compañías. Para consolidar hay que poder eliminarlas, y
  hoy no hay con qué distinguirlas.

Los tres son defectos de diseño que se arreglan construyendo, y los tres aparecen en la propuesta
como trabajo explícito. **El tamaño real del rezago de clasificación y de las partidas con partes
relacionadas solo se podrá medir sobre los datos de producción.**

---

## 6 · Recomendación

**La estructura lo soporta y el camino es más corto de lo que parece**, porque el trabajo caro
—multiempresa, dinero decimal, trazabilidad, captura de IVA y retención— ya está hecho.

**Pero no se debe empezar por el código.** El orden que recomiendo:

1. **Que un contador valide el catálogo reestructurado.** Es gratis en horas de desarrollo y
   condiciona todo lo demás.
2. **Confirmar de dónde salen los saldos iniciales** y a qué fecha de corte.
3. **Decidir si el plan de cuentas es único del grupo o uno por empresa.** Cambia el modelo de
   datos de la Fase 1, y corregirlo después es caro.
4. **Medir sobre los datos de producción** el rezago de clasificación, los conceptos sin
   naturaleza declarada y las operaciones entre empresas del grupo. Nada de eso se puede
   dimensionar con los datos de prueba que hay hoy en la base.
5. Recién entonces, Fases 1 y 2, que son baratas y de riesgo nulo.

Las Fases 1 y 2 se pueden construir **mientras** avanza lo contable, porque no tocan nada y no
cambian ninguna cifra. La Fase 3 no debería empezar hasta que los puntos 1 a 3 estén cerrados.

---

*Diagnóstico técnico sobre el sistema al 23 de octubre de 2026. No es una opinión contable ni
fiscal: los criterios contables y tributarios de Costa Rica debe validarlos un profesional
competente en el país.*
