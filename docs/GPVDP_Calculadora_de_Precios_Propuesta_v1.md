# Calculadora de Precios — Propuesta

**Para:** Coordinación de Gestiones Financieras · GF · GM · GG · Gerencia de Costos
**Fuente analizada:** `029. Costeos Valle de Paz 2023` (13-07-2023), 3 hojas
**Fecha:** 13 de octubre de 2026
**Estado:** PROPUESTA — no se ha construido nada

---

## 0 · Resumen para quien lee una sola página

El modelo de costeo actual es **mucho mejor que el promedio del sector**: tiene lista de
materiales por plan, separa directos de indirectos, y ya piensa en siniestralidad y permanencia.
Eso es una base real sobre la que construir.

Tiene **cinco defectos que cambian decisiones de precio**, y uno de ellos es grave:

1. **Los cuatro planes de suscripción no tienen rentabilidad calculada.** La fila existe y está
   vacía para Gardenia, Tulipán, Olivo y Cedro. Hoy no se sabe si ganan o pierden.
2. **Las dos hojas de indirectos reparten con reglas distintas y contradictorias** (30 %/70 %
   contra 45 %/85 %, y estos últimos suman 130 %).
3. **El costo de un insumo central está vacío**: «Cremación de Adulto → *Averiguar costos*». Los
   planes Otoño y Primavera se están vendiendo con un costo incompleto.
4. **Se costea con precios que incluyen IVA.** Si el IVA soportado es crédito fiscal, el costo
   está inflado ~13 % y los márgenes reportados están subestimados.
5. **No hay inflación ni valor del dinero en el tiempo.** Se cobra una cuota hoy por un servicio
   que se presta dentro de 10 o 20 años, al costo de entonces.

La propuesta es un módulo que no reemplaza el Excel: lo **asciende** a motor de decisión, conectado
a los costos reales que el ERP ya tiene (CxP, Inventario, Nómina, Bancos).

---

## 1 · Lo que hay hoy, leído con precisión

### 1.1 · La estructura del archivo

| Hoja | Qué es | Filas |
|---|---|---|
| `Indirectos Planes` | Gasto indirecto de toda la empresa, por concepto, ene–jun | 142 |
| `Indirectos Servicios` | El mismo gasto, con otra regla de reparto | 142 |
| `Costos` | Lista de materiales (BOM) y costeo de **11 planes** | 379 |

Los 11 planes, en 4 familias:

| Familia | Planes | Modalidad |
|---|---|---|
| Servicios funerarios | Jade, Bronce, Plata, Oro, Diamante | Directo (contado) |
| Cremación | Otoño, Primavera | Directo (contado) |
| Corporativo | Gardenia, Tulipán | Suscripción mensual |
| Hogar | Olivo, Cedro | Suscripción mensual |

### 1.2 · Los planes directos: el modelo funciona

| Plan | Costo | Precio | Margen | % |
|---|---|---|---|---|
| Jade | ₡377.541 | ₡500.000 | ₡122.459 | 24 % |
| Bronce | ₡419.811 | ₡650.000 | ₡230.189 | 35 % |
| Plata | ₡559.236 | ₡850.000 | ₡290.764 | 34 % |
| Oro | ₡936.225 | ₡1.350.000 | ₡413.775 | 31 % |
| Diamante | ₡1.254.607 | ₡2.750.000 | ₡1.495.393 | 54 % |
| Otoño | ₡350.674 | ₡650.000 | ₡299.326 | 46 % |
| Primavera | ₡899.344 | ₡1.250.000 | ₡350.656 | 28 % |

**Lectura de negocio:** el margen NO crece con el precio de forma ordenada (24 % → 35 % → 34 % →
31 % → 54 %). Jade, el plan de entrada, es el de peor margen; Diamante duplica al resto. Eso
puede ser estrategia deliberada —Jade como gancho— o puede ser que el escalonamiento nunca se
diseñó. **Es la primera pregunta que el módulo debe permitir responder de un vistazo.**

### 1.3 · Los planes de suscripción: el hueco grande

| Plan | Costo del servicio | Cuota mensual | Rentabilidad |
|---|---|---|---|
| Gardenia | ₡1.779.566 | ₡4.950 | *(vacío)* |
| Tulipán | ₡1.878.365 | ₡7.800 | *(vacío)* |
| Olivo | ₡1.785.566 | ₡4.950 | *(vacío)* |
| Cedro | ₡1.877.865 | ₡10.000 | *(vacío)* |

El archivo sí tiene las piezas de un modelo actuarial, sueltas al pie de la hoja:

- **Siniestralidad:** 0,47 % y 0,39 %
- **Permanencia promedio** del contrato (la nota dice «base a los 31 mil»)
- **Costo total esperado** por contrato: ₡34.351 (Gardenia)
- **Ingreso total:** ₡178.200 = ₡4.950 × **36 meses**
- **31.000 contratos activos × 9 beneficiarios = 279.000 personas cubiertas**

**El punto que decide todo y hoy está implícito:** con 9 beneficiarios por contrato, la
probabilidad de que *alguno* use el servicio no es 0,47 % — es varias veces eso. Si la
siniestralidad está medida **por beneficiario** y no por contrato, el costo esperado de Gardenia
en 36 meses se acerca o supera los ₡178.200 de ingreso. Si está medida **por contrato**, el plan
es cómodamente rentable.

**No puedo resolverlo desde el archivo: necesito que ustedes confirmen cuál de las dos es.** De
esa sola definición depende si cuatro planes ganan o pierden plata.

### 1.4 · Los indirectos: dos reglas que no se hablan

| | `Indirectos Servicios` | `Indirectos Planes` |
|---|---|---|
| Gasto mensual promedio | ₡122.879.039 | ₡157.569.148 |
| A servicios funerarios | **30 %** | **45 %** |
| A planes | **70 %** | **85 %** |
| Divisor | 360 servicios · 31.000 contratos | 110 planes |
| Resultado unitario | ₡102.399 / servicio · ₡2.775 / contrato-mes | ₡1.217.580 / plan |

Tres problemas:

1. **45 % + 85 % = 130 %.** Se está absorbiendo más gasto del que existe, o las bases son
   distintas y el archivo no lo dice.
2. **El mismo concepto recibe dos valores**: la hoja `Costos` usa ₡102.399 de indirecto para los
   planes directos y ₡1.217.580 para los de suscripción. Son escalas distintas (por servicio
   contra por contrato) mezcladas en la misma fila.
3. **El divisor fijo es una trampa conocida.** Dividir entre «110 planes» o «360 servicios»
   significa que **si baja la venta, sube el costo unitario** — y el sistema pide subir el precio
   justo cuando el mercado se enfría. Es el defecto clásico del costeo por absorción con volumen
   presupuestado.

### 1.5 · Defectos puntuales que hay que corregir sí o sí

- **`Cremación de Adulto`: costo en blanco, nota «Averiguar costos».** Está en la receta de Otoño,
  Primavera, Gardenia, Tulipán, Olivo y Cedro — **seis de los once planes**.
- **Se costea con la columna «Precio Incluye IVA».** Si Valle de Paz es contribuyente y el IVA
  soportado es crédito, el costo real es el de la columna B, no la C.
- **Comisión de vendedor en cero para los cuatro planes de suscripción**, cuando la hoja de
  indirectos muestra ₡8–12 M mensuales de comisiones de ventas.
- **Fórmulas rotas**: `#DIV/0!` en varias celdas y `Total Ventas = 0` en la hoja `Indirectos Planes`.
- **Un cofre aparece con tres proveedores y tres precios** (Nazareno, Robles y Flores, Rafael
  Camacho) sin regla de cuál manda.

---

## 2 · Cómo lo hacen las empresas grandes

Un plan de previsión funeraria **no es un producto, es un seguro**: se cobra hoy por un servicio
futuro, incierto en fecha y en costo. Las compañías que hacen esto bien —aseguradoras, planes de
salud prepagos, memorial parks de EE.UU. y Brasil— usan cinco capas. El Excel actual cubre la
primera y media.

### Capa 1 · Costeo del producto (BOM)
Lista de materiales con cantidad y costo unitario. **Ya existe y está bien hecho.** Lo que suman
las grandes: versionado por fecha (cuánto costaba este plan en enero contra hoy) y costo por
*proveedor preferente* cuando hay varios.

### Capa 2 · Costeo basado en actividades (ABC)
En lugar de repartir los indirectos con un porcentaje, se reparten por el **driver que de verdad
los causa**: la sala de velación por horas de uso, la carroza por kilómetro, el call center por
llamada, cobranza por recibo gestionado. Con ABC, un plan que consume más sala paga más sala.

> Reemplaza el «85 %» por una regla defendible ante una auditoría y ante la Gerencia General.

### Capa 3 · Contribución marginal y punto de equilibrio
Se separa el **costo variable** (cofre, flores, traslado: existe solo si hay servicio) del **costo
fijo** (planilla, alquiler). La contribución marginal —precio menos costo variable— es lo que
paga la estructura. Es la capa que contesta *«¿conviene vender Jade a ₡500.000?»*: si su
contribución es positiva, sí, aunque el margen total parezca bajo.

### Capa 4 · Modelo actuarial (solo suscripción)
Las cuatro variables que gobiernan un plan tipo póliza:

| Variable | Qué es | Dónde está hoy |
|---|---|---|
| **Frecuencia de uso** | Probabilidad de que el contrato genere servicio en el año | 0,47 % · 0,39 % (base sin definir) |
| **Severidad** | Costo del servicio cuando ocurre | ₡1,78–1,88 M ✔ |
| **Permanencia / lapse** | Cuántos meses paga antes de cancelar | «permanencia promedio», suelta |
| **Inflación de costos** | Cuánto subirá el cofre en 15 años | **ausente** |

Con eso se calcula la **prima pura** (frecuencia × severidad), se le suma gasto y utilidad, y sale
la **prima de tarifa**: la cuota que debería cobrarse. Las grandes además segmentan la frecuencia
**por edad del afiliado**, porque un titular de 70 años y uno de 35 no cuestan lo mismo.

### Capa 5 · Precio estratégico
El costo pone el piso, no el precio. Las grandes miran además:
- **Valor de vida del cliente (LTV)** contra costo de adquisición (CAC)
- **Elasticidad**: cuánto cae la venta si el precio sube 10 %
- **Posición competitiva** por plan
- **Escalonamiento intencional** (*good–better–best*): el margen debe crecer con el nivel, y el
  plan intermedio se diseña para ser el más vendido

### Lo que las grandes NUNCA hacen
- Costear con un porcentaje de reparto que nadie puede defender
- Dividir el fijo entre un volumen presupuestado y llamarlo «costo unitario»
- Fijar la cuota de un plan a 20 años sin modelar la inflación del servicio
- Tener un precio sin saber su contribución marginal

---

## 3 · La propuesta: módulo «Calculadora de Precios»

### 3.1 · Principio de diseño

**El costo no se escribe a mano: se lee de donde ya está.** El ERP ya tiene el costo real de los
insumos en CxP (facturas de proveedor con su detalle CABYS), en Inventario (costo de cofres) y en
Nómina (costo de personal). La calculadora se alimenta de ahí y avisa cuando un costo del modelo
se despegó del real.

> Esa es la diferencia entre una hoja de cálculo y un módulo: la hoja hay que actualizarla; el
> módulo se entera.

### 3.2 · Las seis pantallas

**1 · Catálogo de insumos** — Cada insumo con su costo vigente, su proveedor y su historia.
Conectado a CxP: si el cofre #24 se compró la semana pasada a otro precio, la pantalla lo marca.
Resuelve el «un cofre, tres precios» y el «*Averiguar costos*».

**2 · Receta del plan (BOM)** — La hoja `Costos` tal cual, pero viva: insumo, cantidad, costo
unitario, costo extendido. Con **versiones fechadas**, para poder contestar «¿qué costaba
Diamante en enero?».

**3 · Reparto de indirectos (ABC)** — Los conceptos de gasto que el módulo Bancos ya clasifica,
con su driver y su tasa. Reemplaza los porcentajes del Excel y deja ver, concepto por concepto,
cuánto termina en cada plan.

**4 · Modelo actuarial** — Solo para suscripción. Frecuencia por edad, severidad, permanencia,
inflación. Calcula la **prima técnica** y la compara contra la cuota que se cobra hoy.

**5 · Simulador de precio** — El corazón. Se mueve una variable y todo se recalcula en vivo:

> *«Si el cofre sube 12 %, ¿qué pasa con el margen de Oro?»*
> *«¿A qué cuota Gardenia empata en 10 años con inflación del 5 %?»*
> *«Si Jade sube a ₡550.000 y pierdo 8 % de volumen, ¿gano o pierdo?»*

**6 · Tablero de rentabilidad** — La foto para GG: margen por plan, contribución, mezcla de venta,
y las alertas (plan bajo el piso, costo desactualizado, plan sin receta completa).

### 3.3 · Lo que cada gerencia obtiene

| | Pregunta que responde |
|---|---|
| **GF** | ¿Qué margen real deja cada plan después de absorber todo? ¿Cuál es el piso de precio? |
| **Gerencia de Costos** | ¿Qué insumo se disparó y a qué planes golpea? ¿El costo del modelo coincide con lo que paga CxP? |
| **GM** | ¿Cuánto puedo descontar sin perder plata? ¿El escalonamiento empuja al plan que quiero vender? |
| **GG** | ¿Cuánto aporta cada familia al resultado? ¿Dónde está el riesgo si cambia la mezcla? |

### 3.4 · Sinergia con lo que ya existe

| Módulo | Qué aporta a la calculadora |
|---|---|
| **CxP › Gasto por ítem** | Costo real de cada insumo, por CABYS, con su historia de precios |
| **Inventario** | Costo y existencia de cofres y urnas |
| **Nómina** | Costo real por hora del personal de servicio |
| **Bancos › Clasificación** | Los indirectos ya clasificados por partida — insumo directo del ABC |
| **CxC** | Permanencia real de los contratos y cancelaciones: alimenta el modelo actuarial con datos propios, no supuestos |

**CxC es la pieza que nadie más tiene.** Los 31.000 contratos con su historia de pagos son la
fuente para medir la permanencia real y la tasa de cancelación. Ninguna consultora externa puede
calcularlo mejor que ustedes con ese dato en la mano.

---

## 4 · Lo que necesito que decidan antes de construir

Son decisiones de negocio, no técnicas. Ninguna la puedo tomar yo.

1. **La siniestralidad 0,47 %: ¿por contrato o por beneficiario?** De esto depende si los cuatro
   planes de suscripción ganan o pierden. Es la decisión más importante de la lista.
2. **El IVA soportado, ¿es crédito fiscal?** Si lo es, hay que costear con la columna B y todos
   los márgenes publicados suben.
3. **¿Cuál es el margen objetivo por familia?** Hoy van de 24 % a 54 % sin regla visible.
4. **¿Jade es un gancho deliberado?** Su 24 % contra el 54 % de Diamante: ¿estrategia o accidente?
5. **¿Qué inflación anual usamos** para proyectar el costo del servicio futuro?
6. **El costo de «Cremación de Adulto»** — sin ese dato, seis planes tienen el costo incompleto.
7. **¿Los planes de suscripción pagan comisión de venta?** Hoy figuran en cero.

---

## 5 · Cómo lo construiría, por etapas

Cada etapa entrega algo usable por sí sola. No hay que esperar al final.

| Etapa | Qué entrega | Por qué primero |
|---|---|---|
| **1 · Catálogo + Receta** | El Excel, vivo y conectado a CxP | Sin costo confiable, lo demás es decoración |
| **2 · Contribución marginal** | Variable contra fijo, y el piso de precio de cada plan | Es la cifra que falta para negociar un descuento sin perder plata |
| **3 · ABC** | Reparto de indirectos por driver | Reemplaza el 85 % por algo defendible |
| **4 · Actuarial** | Prima técnica contra cuota cobrada | Cierra el hueco de los cuatro planes de suscripción |
| **5 · Simulador + Tablero** | Escenarios y la foto para GG | Vale cuando las cuatro capas de abajo son confiables |

**Mi recomendación de orden:** empezar por la **etapa 2** en paralelo con la 1. La contribución
marginal se puede calcular con los datos que ya están en el Excel y responde hoy la pregunta que
más plata mueve: *cuál es el precio mínimo al que conviene vender cada plan*.

---

## 6 · Lo que este documento NO hace

- No recalcula los precios: para eso hacen falta las siete decisiones de la sección 4.
- No afirma que los planes de suscripción pierdan plata. Afirma que **hoy no se sabe**, y que la
  respuesta depende de una definición que está pendiente.
- No reemplaza el criterio comercial. El costo pone el piso; el precio lo pone el mercado y la
  estrategia.
