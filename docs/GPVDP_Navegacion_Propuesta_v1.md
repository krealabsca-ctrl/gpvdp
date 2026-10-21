# Navegación — propuesta

**Pedido:** «en la operativa diaria mantener esta línea abierta roba espacio, más cuando estoy en
un módulo ya interno».

---

## 1 · El problema, medido

La navegación son dos piezas fijas que nunca se van:

| Pieza | Ancho | Qué muestra |
|---|---|---|
| **Riel de módulos** (iconos) | 64 px | Los 8 módulos disponibles |
| **Sidebar contextual** | 240 px | Las páginas del módulo activo |
| | **304 px** | **siempre ocupados** |

En la pantalla típica de oficina (1366 px) eso es el **22 % del ancho**, y no cambia nunca: ni
cuando estás clasificando movimientos en una tabla de 10 columnas, ni cuando ya sabés de memoria
dónde está todo.

**Y el problema crece con el sistema.** Hoy hay 8 módulos con 57 páginas:

| Módulo | Páginas |
|---|---|
| Cuentas por pagar | 16 |
| Bancos | 13 |
| Inventario | 9 |
| Recursos Humanos | 8 |
| Cuentas por cobrar | 7 |
| Calculadora de precios | 2 |
| Configuración | 2 |

Bancos con 13 entradas ya no entra cómodo en una pantalla de portátil sin hacer scroll en el
propio menú. Cuando CxP llegue a 20, la sidebar va a necesitar su propia barra de desplazamiento
mientras le roba un quinto de la pantalla a la tabla que estás leyendo.

---

## 2 · Cómo lo resuelven los sistemas grandes

Cuatro patrones, y lo que cada uno enseña:

| Sistema | Qué hace | Lo que se aprende |
|---|---|---|
| **SAP Fiori** | Navegación lateral **colapsable**, y recuerda el estado por usuario | El experto la cierra y no la vuelve a abrir; el nuevo la deja abierta. No hay que elegir por ellos |
| **Salesforce Lightning** | Barra **horizontal** arriba con pestañas; cero sidebar | Gana ancho, pero no escala a 16 páginas por módulo sin menús desplegables anidados |
| **Dynamics 365** | Sitemap lateral colapsable + **búsqueda global** siempre visible | Con muchas pantallas, buscar le gana a navegar |
| **Linear · Notion · Stripe** | Sidebar colapsable con **atajo de teclado** + **paleta de comandos** (Ctrl+K) | El usuario frecuente deja de usar el menú: escribe a dónde va |

**El patrón común de los cuatro**: la navegación persistente es para *descubrir*; una vez que el
usuario sabe dónde está todo, estorba. Por eso ninguno la deja fija.

**Lo que ninguno hace**, y conviene nombrarlo: esconder la navegación detrás de una hamburguesa
que hay que abrir en cada uso. Eso cambia «ocupa espacio» por «cuesta dos clics cada vez», que en
operación diaria es peor.

---

## 3 · La propuesta: cuatro cambios, en orden de impacto

### 3.1 · Colapsar la sidebar, con memoria por usuario *(el que resuelve tu pedido)*

Un botón en el borde de la sidebar y el atajo **Ctrl + B**. Al colapsar, la sidebar de 240 px se
convierte en **nada** y el riel de iconos (64 px) queda solo.

**Se gana 240 px — el 18 % de la pantalla — sin perder navegación**: el riel sigue ahí, y pasar el
mouse sobre el icono del módulo despliega sus páginas en un panel flotante que se cierra solo al
elegir. Es lo que hace Fiori.

> **La memoria es la mitad de la función.** El estado se guarda por usuario, así que quien trabaja
> todo el día en Clasificar la colapsa una vez y la encuentra colapsada mañana. Si hay que
> cerrarla en cada ingreso, nadie la usa.

### 3.2 · Auto-colapso en las pantallas anchas

Las pantallas de trabajo intensivo —Clasificar, Bandeja de CxP, Cartera de CxC, Exportar— nacen
con la sidebar colapsada **la primera vez** que el usuario entra. Si él la abre, se respeta su
decisión de ahí en adelante.

Es la diferencia entre un sistema que impone y uno que propone un valor por defecto sensato.

### 3.3 · Paleta de comandos (Ctrl + K)

Escribir «clasif» y llegar a Bancos › Clasificar sin tocar el menú.

Con 57 páginas, esto deja de ser un lujo: es más rápido que recordar en qué módulo vive cada
pantalla. Es, además, lo que resuelve el problema que ya tuvimos con descubribilidad —«Horas extra
y vacaciones existían pero nadie las hallaba»—: buscar «vacaciones» las encuentra aunque el menú
las llame de otro modo.

**Alcance**: solo navega, no ejecuta acciones. Y solo lista lo que el usuario tiene permiso de ver,
igual que el menú.

### 3.4 · Migas de pan en la barra superior

Cuando la sidebar está colapsada hace falta saber dónde se está parado. Una línea en el TopBar:

```
Bancos  ›  Clasificar
```

Ocupa alto que ya existe y reemplaza la función de orientación que hoy cumple la sidebar abierta.

---

## 4 · Lo que NO propongo, y por qué

| Idea | Por qué no |
|---|---|
| **Pasar todo a menú horizontal** (estilo Salesforce) | Con 16 páginas en CxP haría falta anidar desplegables, y un submenú que se cierra solo al mover el mouse es peor que la sidebar |
| **Esconder el riel de módulos también** | Son 64 px y es lo único que permite cambiar de módulo de un clic. El costo es bajo y el beneficio alto |
| **Hamburguesa que tapa el contenido** | Cambia «ocupa espacio» por «dos clics cada vez». En operación diaria se paga todos los días |
| **Que la sidebar se abra sola al pasar el mouse** | Un panel que aparece sin que lo pidas mientras leés una tabla es peor que uno fijo |

---

## 5 · Qué habría que construir

| | Qué toca | Tamaño |
|---|---|---|
| **1 · Colapsar + memoria** | `Sidebar.tsx`, `AppShell.tsx`, preferencia por usuario | Chico |
| **2 · Panel flotante del riel** | `ModuleRail.tsx` | Chico |
| **3 · Auto-colapso por pantalla** | Una marca en el registro de navegación (`nav.ts`) | Muy chico |
| **4 · Migas de pan** | `TopBar.tsx`, derivadas de `nav.ts` | Muy chico |
| **5 · Paleta de comandos** | Componente nuevo, se alimenta de `nav.ts` y de los permisos | Mediano |

**Lo bueno**: los cinco se alimentan del registro de navegación que ya existe (`nav.ts` es fuente
única de módulos y páginas), así que ninguno agrega una lista que haya que mantener aparte. Un
módulo nuevo —como la Calculadora de Precios— aparece solo en el menú, en las migas y en la
paleta.

**Mi recomendación**: construir **1, 2 y 4 juntos** —son el pedido concreto y suman menos de un
día— y dejar la paleta de comandos para después, cuando se vea si con la sidebar colapsable alcanza.

---

## 6 · Lo que esta propuesta no decide

- **Si el auto-colapso debe aplicar a todas las pantallas anchas o solo a algunas.** Eso se sabe
  viendo cuáles usás con la sidebar cerrada después de la primera semana.
- **Si el riel debería mostrar etiquetas además de iconos.** Hoy son solo iconos con tooltip; con
  8 módulos funciona, con 15 habría que revisarlo.
