# Módulo de Inventario — Propuesta v1.0

> Estado: **propuesta**, pendiente de decisiones del Director Financiero. Nada construido.
> Fecha: 2026-08-22. Alcance pedido: control de cofres/ataúdes, urnas y las demás categorías,
> en varias sedes. Volumen declarado por el usuario: 40–50 unidades en stock hoy, hasta 100–150
> entre todas las sedes; urnas se piden todas las semanas.

---

## 1. Lo que dicen los datos que ya están en el sistema

Medido el 2026-08-22 sobre la base real, no estimado.

| Hecho medido | Valor |
|---|---|
| Tablas de inventario en el esquema | **0** (se parte de cero, sin deuda técnica) |
| Facturas de CxP con líneas de detalle | **0 de 4.542** — solo se guarda el total |
| Facturas clasificadas como producto físico | **51 de 4.542** |
| Sedes cargadas en la dimensión del gasto (`sede`) | **0** |
| Contratos de CxC | **0** (el módulo está construido pero vacío) |
| Registro de servicio funerario prestado | **no existe ninguna tabla** |

Gasto real por partida (agosto 2026 es el único mes con ≥90 % clasificado, así que es el único
comparable):

| Partida | Movs | Gasto |
|---|---:|---:|
| Cofres / Ataúd | 6 | ₡10.855.429,75 |
| Patologías | 24 | ₡10.003.950,00 |
| Flores | 9 | ₡4.262.096,51 |
| Suministros de Velación | 15 | ₡1.483.375,41 |
| Cremaciones | 3 | ₡751.450,00 |
| Lápidas | 2 | ₡345.652,01 |
| Urnas | 3 | ₡323.971,00 |

Facturas individuales de cofres: entre **₡70.000 y ₡423.750**. Proveedor concentrado:
MEGASOLUCIONES aporta 36 de las 51 facturas de producto (enero–julio 2026).

### Los cuatro hallazgos que condicionan el diseño

1. **El evento que descarga el stock no existe.** No hay registro del servicio funerario prestado.
   CxC es cartera de asociados (planes prepagados) y está vacío. Sin ese evento, un módulo de
   inventario es un contador que alguien tiene que corregir a mano, y se desactualiza en una semana.
   **Es la decisión número uno.**

2. **La contabilidad no puede alimentar el inventario.** Las facturas no tienen líneas: se sabe
   cuánto se pagó y a quién, nunca qué ni cuántos. El inventario necesita su propio catálogo.

3. **La partida contable no identifica el producto.** Verificado: `CORAL SERVICIOS DE ALIMENTACIÓN`
   y `TICAFLOR Distribuidora de Flores` están clasificados bajo «Cofres / Ataúd». La partida sirve
   para el gasto, no para el stock.

4. **El banco tampoco sirve como fuente.** Los pagos salen agrupados en SINPE masivos
   (₡3.413.730 / ₡3.264.231 en agosto): un pago cubre varias facturas de varios proveedores.

---

## 2. Cómo lo resuelven los grandes

### Industria funeraria

- **El problema es el surtido, no el volumen.** Hay que ofrecer variedad de modelos con demanda baja
  y errática por modelo. Nadie predice cuántos cofres de roble se venden en octubre.
- **Consignación.** El proveedor deja el producto en la funeraria y cobra cuando se usa. Es práctica
  extendida en el sector justamente porque el surtido amplio inmoviliza mucho capital.
- **Los modelos de exhibición no son stock vendible.** La sala de exhibición se cuenta aparte.
- **Rotación por modelo** para detectar lo que no se mueve, y conteos periódicos en vez de un
  inventario general anual.

### Retail / Amazon

- **ABC × XYZ**: clasificar por valor (A/B/C) y por variabilidad de la demanda (X/Y/Z), y darle a
  cada clase su política de stock de seguridad y frecuencia de revisión.
- **Punto de reorden** = consumo diario promedio × días de entrega + stock de seguridad.
- **Conteo cíclico ponderado**: los artículos A se cuentan varias veces al año, los C una vez.

### Lo que NO se aplica a este caso

Con 100–150 unidades, pronosticar estadísticamente es ruido: la muestra por modelo es demasiado
chica. Amazon aporta el **marco de decisión** (clasificar, fijar mínimos, contar por ciclos), no la
maquinaria. El valor está en saber qué hay, dónde está, y que el sistema avise cuándo pedir.

---

## 3. La propuesta

### 3.1 Dos formas de contar, según el artículo

| | Cofres / ataúdes | Urnas, flores, suministros |
|---|---|---|
| Control | **por unidad** (cada cofre es una fila) | **por cantidad** |
| Por qué | ₡70k–₡424k cada uno, 40–150 unidades, se trasladan entre sedes, hay que saber cuál se usó en cuál servicio | se piden semanalmente, valen poco, no tienen identidad propia |
| Costeo | costo específico de esa unidad (exacto) | costo promedio ponderado |
| Identificación | número de unidad (placa o etiqueta) | solo el código de artículo |

Es la misma distinción que hace la industria: trazabilidad unitaria para el activo caro, cantidad
para el consumible. Un solo mecanismo para los dos casos obliga a elegir entre perder trazabilidad
del cofre o llenar de burocracia la urna.

### 3.2 Modelo de datos (5 tablas nucleares)

| Tabla | Qué guarda |
|---|---|
| `inv_categoria` | El árbol de categorías (Cofres, Urnas, Lápidas, Suministros…). |
| `inv_articulo` | El catálogo: código, nombre, categoría, **modo de control** (UNIDAD / CANTIDAD), proveedor habitual, mínimo y máximo por sede. |
| `inv_unidad` | Solo para los de modo UNIDAD: una fila por cofre físico, con su estado y su sede. |
| `inv_movimiento` | **El libro mayor**: toda entrada, salida, traslado y ajuste. Append-only. |
| `inv_conteo` | Los conteos cíclicos y sus diferencias. |

**Decisión de arquitectura: la existencia se DERIVA del libro mayor, no se guarda.** Es el mismo
principio que ya usa Bancos con el saldo diario. Un campo `cantidad_actual` que se actualiza a mano
se desincroniza y nadie sabe cuál de los dos números es el verdadero; derivándolo, la existencia
siempre cuadra con su historia y cada unidad tiene su trazabilidad completa sin trabajo extra.

### 3.3 Estados de una unidad

```
        compra                 asignación            servicio prestado
  (nada) ────▶ DISPONIBLE ──────▶ RESERVADA ──────────▶ USADA (fin)
                  │  ▲                │
      traslado    │  │                └── se libera ──▶ DISPONIBLE
                  ▼  │
            EN_TRÁNSITO ── recibida ─┘

  DISPONIBLE ──▶ EXHIBICIÓN   (modelo de sala: no es stock vendible)
  DISPONIBLE ──▶ DAÑADA       (baja con motivo)
  DISPONIBLE ──▶ DEVUELTA     (al proveedor)
```

`EN_TRÁNSITO` es el estado que evita la pérdida entre sedes: la unidad sale de la sede A y **no
aparece en B hasta que alguien la recibe**. Sin ese estado intermedio, una unidad extraviada en el
camino simplemente desaparece del sistema sin que nadie tenga que responder por ella.

### 3.4 El evento de servicio (la decisión número uno)

Propongo un registro mínimo de **servicio prestado**: número, sede, fecha, nombre del fallecido o
del contrato, y qué artículos consumió. Es lo que descarga el inventario y lo que después permite
calcular el margen real de cada servicio.

Sin esto, el módulo no funciona: alguien tendría que registrar cada salida a mano y sin referencia
a nada, y el primer día que no lo haga el inventario deja de servir.

### 3.5 Consignación (soportarla desde el día uno)

Con 150 cofres a un promedio conservador de ₡150.000 hay ~₡22 millones inmovilizados. Si el
proveedor consigna, ese capital no es de la empresa. La unidad lleva `es_consignada` y su proveedor;
al usarla, nace la cuenta por pagar en CxP.

Ponerlo después obliga a reclasificar unidades ya cargadas y a decidir retroactivamente de quién era
cada una — exactamente el reproceso que se evitó en Bancos con las dimensiones.

### 3.6 Reposición

Mínimo y máximo **por artículo y por sede**, fijados a mano al inicio. El punto de reorden se
calcula con el consumo real que el módulo va acumulando:

```
punto de reorden = consumo semanal promedio × semanas de entrega + colchón
```

Los primeros dos o tres meses no hay historia suficiente, así que el mínimo lo pone el usuario y la
pantalla dice explícitamente que todavía no hay datos para sugerirlo. Después el sistema propone y
el usuario aprueba. **Nunca pide solo.**

### 3.7 Costeo

- Modo UNIDAD → **costo específico**: el costo de esa unidad es lo que se pagó por ella.
- Modo CANTIDAD → **promedio ponderado**, recalculado en cada entrada.

---

## 4. Pantallas

| Pantalla | Contesta |
|---|---|
| **Existencias** | Qué hay, en qué sede, cuánto vale. Semáforo de bajo mínimo. |
| **Entrada** | Llegó mercadería: de qué proveedor, a qué sede, con referencia a la factura de CxP. |
| **Salida por servicio** | Se usó en tal servicio (descarga el stock). |
| **Traslados** | Enviar y recibir entre sedes, con el estado en tránsito. |
| **Conteo** | Hoja de conteo por sede y categoría, y la diferencia contra lo que el sistema dice. |
| **Reposición** | Qué hay que pedir esta semana, agrupado por proveedor. |
| **Análisis** | Rotación por modelo, lo que no se mueve, capital inmovilizado por sede. |

---

## 5. Lo que hace falta decidir (no lo puedo inventar)

1. **¿Quién registra el servicio prestado y cuándo?** ¿El de mostrador al momento, o alguien al
   cierre del día?
2. **¿Hay cofres en consignación hoy?** ¿Con qué proveedores?
3. **¿Cuántas sedes tienen bodega propia**, y cuáles solo exhiben?
4. **¿Los cofres se numeran hoy** (placa, etiqueta, factura del proveedor), o hay que crear el
   número al recibirlos?
5. **¿El precio de venta vive en este módulo** o ya está en otra parte (lista de precios, contrato)?
6. **¿Las urnas se piden a un proveedor fijo** todas las semanas, o se cotiza cada vez?
7. **¿Se controla también lo que consume el servicio pero no es producto** (flores, suministros de
   velación), o solo cofres y urnas?

---

## 6. Fases propuestas

1. **Catálogo y existencias.** Categorías, artículos, unidades, entradas, salidas y traslados. Es el
   corazón: sin esto no hay nada.
2. **Conteo y reposición.** Conteo cíclico, diferencias, mínimos y qué pedir.
3. **Consignación y margen.** Unidades consignadas con su enlace a CxP, y el margen real por
   servicio.

---

## 7. Lo que NO propongo, y por qué

| No incluido | Razón |
|---|---|
| Pronóstico estadístico de demanda | Con 150 unidades y demanda errática por modelo, el pronóstico sería ruido presentado como certeza. |
| Códigos de barras / RFID | Vale la pena cuando el conteo manual duele. Con 150 unidades todavía no duele. |
| Valuación contable automática del inventario | Es una decisión contable, no de sistema. El módulo da el dato; quién lo asienta y cómo lo decide el Director Financiero. |
| Reserva automática de unidad para contratos prepagados | Un plan prepagado compra un **nivel de servicio**, no un cofre específico. Reservar la unidad inmovilizaría stock por años. |

---

## Guardarraíl

El inventario valorizado toca el activo de la empresa. Este módulo **registra lo que hay**: no se
construirán funciones para inflar existencias, sostener stock inexistente ni valorizar por encima
del costo real. Si un conteo da una diferencia, la diferencia se muestra y se explica; no se ajusta
en silencio.
