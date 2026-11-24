/**
 * Bancos → Formatos de banco (/formatos).
 *
 * EL PEDIDO (punto 2 de la revisión comercial): los seis formatos de banco viven en el código, así
 * que cada banco nuevo es un ticket de desarrollo, un build y un despliegue. Es el techo de
 * cuántos clientes se pueden atender sin crecer el equipo.
 *
 * EL ORDEN DE ESTA PANTALLA NO ES EL DE LOS DATOS, ES EL DEL TRABAJO. Primero se sube el archivo,
 * después se configura. Al revés —un formulario con cinco campos numéricos en blanco— se está
 * pidiendo contar columnas de memoria, y la columna 4 de un Excel es la E. Un formato con el
 * débito y el crédito cambiados lee el archivo entero al revés y nadie lo nota hasta que el cuadre
 * del mes no cierre.
 *
 * Por eso: se sube, se ve la grilla con los números de columna encima, el sistema propone, la
 * persona corrige, y «Probar» muestra los movimientos ya leídos ANTES de que el formato exista.
 */

import { useMemo, useRef, useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  Select,
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useBorrarFormatoBanco,
  useFormatosBanco,
  useGuardarFormatoBanco,
  useProbarFormatoBanco,
} from "@/features/bancos/hooks";
import {
  adivinarColumnas,
  filaDeEncabezadoProbable,
  proponerSenales,
  type PapelColumna,
} from "@/features/bancos/formatoAdivinado";
import type { FormatoBanco, PruebaDeFormato, SenalEncabezado } from "@/api/formatosBanco";

/** Lo que se está editando. Las columnas van como número; -1 es «no existe». */
interface Borrador {
  id?: string;
  nombre: string;
  activo: boolean;
  texto_identificador: string;
  col_fecha: number;
  col_documento: number;
  col_descripcion: number;
  col_debito: number;
  col_credito: number;
  formato_fecha: string;
  senales: SenalEncabezado[];
}

const VACIO: Borrador = {
  nombre: "",
  activo: true,
  texto_identificador: "",
  col_fecha: -1,
  col_documento: -1,
  col_descripcion: -1,
  col_debito: -1,
  col_credito: -1,
  formato_fecha: "",
  senales: [],
};

/** Los cinco papeles, con la etiqueta que se muestra y si es obligatorio. */
const PAPELES: { papel: PapelColumna; campo: keyof Borrador; label: string; obligatorio: boolean }[] = [
  { papel: "fecha", campo: "col_fecha", label: "Fecha del movimiento", obligatorio: true },
  { papel: "descripcion", campo: "col_descripcion", label: "Descripción / concepto", obligatorio: true },
  { papel: "debito", campo: "col_debito", label: "Débito (lo que sale)", obligatorio: true },
  { papel: "credito", campo: "col_credito", label: "Crédito (lo que entra)", obligatorio: true },
  { papel: "documento", campo: "col_documento", label: "Referencia del banco", obligatorio: false },
];

export function FormatosBancoPage() {
  const toast = useToast();
  const puedeEditar = useTienePermiso()("bancos.formatos");

  const listaQ = useFormatosBanco();
  const guardar = useGuardarFormatoBanco();
  const borrar = useBorrarFormatoBanco();
  const probar = useProbarFormatoBanco();

  const [abierto, setAbierto] = useState(false);
  const [b, setB] = useState<Borrador>(VACIO);
  const [archivo, setArchivo] = useState<File | null>(null);
  const [filas, setFilas] = useState<string[][]>([]);
  const [filaEncabezado, setFilaEncabezado] = useState(-1);
  const [prueba, setPrueba] = useState<PruebaDeFormato | null>(null);
  const inputArchivo = useRef<HTMLInputElement>(null);

  const formatos = listaQ.data?.items ?? [];
  const opcionesFecha = listaQ.data?.formatos_fecha ?? [];
  const deFabrica = listaQ.data?.bancos_del_codigo ?? [];

  // Cuántas columnas tiene la fila más ancha: es el rango de los selectores.
  //
  // El `?? []` no sobra: un estado de cuenta real trae filas en blanco entre el título y el
  // encabezado, y una fila vacía puede llegar como null. El servidor ya las normaliza, pero esta
  // pantalla se cae ENTERA si una se le cuela, así que no depende de eso.
  const anchoMax = useMemo(
    () => filas.reduce((m, f) => Math.max(m, (f ?? []).length), 0),
    [filas],
  );

  function set<K extends keyof Borrador>(k: K, v: Borrador[K]) {
    setB((prev) => ({ ...prev, [k]: v }));
    setPrueba(null); // lo que se vio ya no corresponde a lo que hay ahora
  }

  function limpiar() {
    setB(VACIO);
    setArchivo(null);
    setFilas([]);
    setFilaEncabezado(-1);
    setPrueba(null);
    setAbierto(false);
    if (inputArchivo.current) inputArchivo.current.value = "";
  }

  /**
   * Sube el archivo y, con lo que vuelve, propone todo lo que se puede proponer.
   *
   * Se usa `probar` con un formato deliberadamente incompleto: el servidor devuelve las primeras
   * filas aunque no reconozca nada, y esas filas son justamente lo que hace falta para configurar.
   */
  function elegirArchivo(f: File) {
    setArchivo(f);
    setPrueba(null);
    probar.mutate(
      { formato: { ...b, nombre: b.nombre || "(sin nombre)" }, archivo: f },
      {
        onSuccess: (r) => {
          setFilas(r.filas ?? []);
          const i = filaDeEncabezadoProbable(r.filas ?? []);
          if (i < 0) {
            toast.info("No se reconoció el encabezado solo: marcalo vos en la grilla.");
            return;
          }
          // EDITANDO UN FORMATO QUE YA EXISTE: se marca la fila de títulos —hace falta para
          // dibujar la grilla y para poner el nombre de cada columna en los selectores— pero NO
          // se tocan las columnas ni las señales guardadas (23-oct-2026).
          //
          // Antes se adivinaba igual y se pisaba todo. Eso borra en silencio cualquier corrección
          // hecha a mano, y hay bancos que la exigen: en Scotiabank los títulos están corridos
          // respecto de los datos —la columna titulada «Descripcion» trae el número de
          // documento—, así que la configuración correcta va a propósito CONTRA lo que adivina
          // el sistema. Subir el archivo para probar no puede deshacerla.
          if (b.id) {
            setFilaEncabezado(i);
            setPrueba(null);
            toast.info(`Fila ${i + 1} marcada como los títulos. Tu configuración no se tocó.`);
            return;
          }
          usarComoEncabezado(i, r.filas);
          toast.info(`Se reconoció la fila ${i + 1} como el encabezado. Revisá las columnas abajo.`);
        },
        // Un formato incompleto da 400 y eso es esperable: igual se queda el archivo cargado.
        onError: () => {
          setFilas([]);
          toast.error("No se pudo leer el archivo. ¿Es un Excel del banco?");
        },
      },
    );
  }

  /**
   * Marca una fila como el encabezado y rellena columnas y señales desde ella.
   *
   * Pisa lo que haya, y acá sí corresponde: se llama desde el clic en una fila de la grilla, que
   * es un gesto explícito —«los títulos son estos»— y lo que se espera de él es justamente que
   * vuelva a completar todo. Lo que no puede pisar nada es el simple hecho de subir un archivo
   * para probar un formato ya guardado; eso se resuelve en `elegirArchivo`.
   */
  function usarComoEncabezado(i: number, desde?: string[][]) {
    const fuente = desde ?? filas;
    const fila = fuente[i] ?? [];
    setFilaEncabezado(i);
    const cols = adivinarColumnas(fila);
    setB((prev) => ({
      ...prev,
      col_fecha: cols.fecha,
      col_documento: cols.documento,
      col_descripcion: cols.descripcion,
      col_debito: cols.debito,
      col_credito: cols.credito,
      senales: proponerSenales(fila, cols),
    }));
    setPrueba(null);
  }

  function probarAhora() {
    if (!archivo) {
      toast.error("Subí primero el estado de cuenta con el que querés probar.");
      return;
    }
    probar.mutate(
      { formato: b, archivo },
      {
        onSuccess: (r) => {
          setPrueba(r);
          if (r.filas?.length) setFilas(r.filas);
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  function guardarAhora() {
    guardar.mutate(b, {
      onSuccess: () => {
        toast.success(`Formato «${b.nombre}» guardado. Los próximos archivos de ese banco se van a reconocer solos.`);
        limpiar();
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function editar(f: FormatoBanco) {
    setB({ ...f });
    setPrueba(null);
    setFilas([]);
    setFilaEncabezado(-1);
    setArchivo(null);
    setAbierto(true);
    if (typeof window !== "undefined") window.scrollTo({ top: 0, behavior: "smooth" });
  }

  const faltanColumnas = PAPELES.filter((p) => p.obligatorio && (b[p.campo] as number) < 0);
  const puedeGuardar =
    b.nombre.trim() !== "" && b.formato_fecha !== "" && faltanColumnas.length === 0 && b.senales.length > 0;

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Formatos de banco"
        description="Enseñarle al sistema a leer el estado de cuenta de un banco que no viene de fábrica."
        actions={
          puedeEditar && (
            <Button variant={abierto ? "secondary" : "primary"} onClick={() => (abierto ? limpiar() : setAbierto(true))}>
              {abierto ? "Cerrar" : "+ Agregar un banco"}
            </Button>
          )
        }
      />

      {/* Los seis de fábrica: para que nadie configure uno que ya existe, y para que se entienda
          que un formato propio NO los reemplaza —se prueba antes, y si no calza, ellos siguen—. */}
      {deFabrica.length > 0 && (
        <Card>
          <CardContent className="flex flex-wrap items-center gap-2 pt-5">
            <span className="text-sm text-content-muted">Ya vienen reconocidos:</span>
            {deFabrica.map((n) => (
              <Badge key={n} tone="neutral">{n}</Badge>
            ))}
            <span className="text-xs text-content-muted">
              · No hace falta configurarlos. Lo que agregués acá se prueba antes que ellos.
            </span>
          </CardContent>
        </Card>
      )}

      {abierto && puedeEditar && (
        <Card>
          <CardHeader>
            <CardTitle>{b.id ? `Editar «${b.nombre}»` : "Agregar un banco"}</CardTitle>
          </CardHeader>
          <CardContent className="flex flex-col gap-5">
            {/* PASO 1 — el archivo primero. Todo lo demás se configura mirándolo. */}
            <div className="flex flex-col gap-2">
              <p className="text-sm font-medium text-content">1 · Subí un estado de cuenta de ese banco</p>
              <input
                ref={inputArchivo}
                type="file"
                accept=".xlsx,.xls"
                onChange={(e) => {
                  const f = e.target.files?.[0];
                  if (f) elegirArchivo(f);
                }}
                className="text-sm text-content file:mr-3 file:rounded-lg file:border-0 file:bg-accent file:px-3 file:py-1.5 file:text-sm file:text-white"
              />
              <p className="text-xs text-content-muted">
                No se guarda ni se importa nada: solo se lee para que puedas ver las columnas.
              </p>
            </div>

            {/* PASO 2 — la grilla con los números de columna arriba. Es lo que evita contar a ciegas. */}
            {filas.length > 0 && (
              <div className="flex flex-col gap-2">
                <p className="text-sm font-medium text-content">
                  2 · Marcá cuál es la fila de los títulos
                </p>
                <div className="overflow-x-auto rounded-lg border border-border">
                  <table className="w-full text-xs">
                    <thead>
                      <tr className="bg-surface-raised">
                        <th className="px-2 py-1 text-left font-medium text-content-muted">fila</th>
                        {Array.from({ length: anchoMax }, (_, i) => (
                          <th key={i} className="px-2 py-1 text-left font-mono font-medium text-accent">
                            {i}
                          </th>
                        ))}
                      </tr>
                    </thead>
                    <tbody>
                      {filas.map((fila, i) => (
                        <tr
                          key={i}
                          onClick={() => usarComoEncabezado(i)}
                          className={
                            i === filaEncabezado
                              ? "cursor-pointer bg-accent/10 font-medium text-content"
                              : "cursor-pointer text-content-muted hover:bg-surface-raised"
                          }
                          title="Usar esta fila como encabezado"
                        >
                          <td className="px-2 py-1 font-mono">{i + 1}</td>
                          {Array.from({ length: anchoMax }, (_, c) => (
                            <td key={c} className="max-w-40 truncate px-2 py-1">{(fila ?? [])[c] ?? ""}</td>
                          ))}
                        </tr>
                      ))}
                    </tbody>
                  </table>
                </div>
                <p className="text-xs text-content-muted">
                  Los números de arriba son las columnas. Al marcar la fila de títulos, las de abajo
                  se completan solas —revisalas igual—.
                </p>
              </div>
            )}

            {/* PASO 3 — los campos. Cada selector muestra el TEXTO del encabezado, no solo el número. */}
            <div className="flex flex-col gap-3">
              <p className="text-sm font-medium text-content">3 · Qué viene en cada columna</p>
              <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
                <Input
                  label="Nombre del banco *"
                  value={b.nombre}
                  onChange={(e) => set("nombre", e.target.value)}
                  placeholder="Ej.: Lafise"
                />
                <Select
                  label="Cómo viene la fecha *"
                  placeholder="Elegí…"
                  value={b.formato_fecha}
                  onChange={(e) => set("formato_fecha", e.target.value)}
                  options={opcionesFecha.map((o) => ({ value: o.clave, label: `${o.clave} — ${o.ejemplo}` }))}
                />
                <Input
                  label="Texto que identifica al archivo"
                  value={b.texto_identificador}
                  onChange={(e) => set("texto_identificador", e.target.value)}
                  placeholder="Ej.: banco lafise"
                />
                {PAPELES.map((p) => (
                  <Select
                    key={p.campo}
                    label={p.obligatorio ? `${p.label} *` : p.label}
                    value={String(b[p.campo] as number)}
                    onChange={(e) => set(p.campo, Number(e.target.value) as never)}
                    options={[
                      { value: "-1", label: p.obligatorio ? "— falta elegir —" : "— no tiene —" },
                      ...Array.from({ length: Math.max(anchoMax, 1) }, (_, i) => ({
                        value: String(i),
                        label: etiquetaColumna(i, filas[filaEncabezado]),
                      })),
                    ]}
                  />
                ))}
              </div>
            </div>

            {/* Las señales: lo que distingue este archivo del de otro banco. */}
            <div className="flex flex-col gap-2">
              <p className="text-sm font-medium text-content">4 · Cómo se reconoce el archivo</p>
              <p className="text-xs text-content-muted">
                Hacen falta al menos dos de estas, o una más el texto de arriba. Si no, este formato
                reclamaría los archivos de otros bancos.
              </p>
              {b.senales.map((s, i) => (
                <div key={i} className="flex flex-wrap items-end gap-2">
                  <Select
                    label="En la columna"
                    value={String(s.columna)}
                    // Al elegir la columna, el texto se completa SOLO con lo que dice esa columna
                    // en la fila de títulos (23-oct-2026).
                    //
                    // El caso que lo motivó: en el estado de cuenta de Scotiabank los títulos
                    // están corridos respecto de los datos —la columna titulada «Descripcion»
                    // trae el número de documento—, así que quien configuró las señales mirando
                    // los DATOS las escribió cruzadas y el formato no reconoció nada. Las señales
                    // se comparan contra los TÍTULOS, y escribirlas a mano es justo donde eso se
                    // confunde. Completarlas desde la fila marcada quita el error de raíz.
                    onChange={(e) => {
                      const col = Number(e.target.value);
                      const n = [...b.senales];
                      n[i] = { columna: col, texto: tituloDe(col, filas[filaEncabezado]) || s.texto };
                      set("senales", n);
                    }}
                    options={Array.from({ length: Math.max(anchoMax, 1) }, (_, c) => ({
                      value: String(c),
                      label: etiquetaColumna(c, filas[filaEncabezado]),
                    }))}
                  />
                  <Input
                    label="aparece el texto"
                    value={s.texto}
                    onChange={(e) => {
                      const n = [...b.senales];
                      n[i] = { columna: s.columna, texto: e.target.value };
                      set("senales", n);
                    }}
                    placeholder="debitos"
                  />
                  <Button
                    variant="ghost"
                    size="sm"
                    onClick={() => set("senales", b.senales.filter((_, j) => j !== i))}
                  >
                    Quitar
                  </Button>
                </div>
              ))}
              <div>
                <Button
                  variant="secondary"
                  size="sm"
                  // Nace con el título de la columna 0 ya puesto, por lo mismo que el selector:
                  // una señal en blanco invita a escribirla a mano, que es donde se cruza.
                  onClick={() =>
                    set("senales", [
                      ...b.senales,
                      { columna: 0, texto: tituloDe(0, filas[filaEncabezado]) },
                    ])
                  }
                >
                  + Agregar una señal
                </Button>
                {filaEncabezado >= 0 && (
                  <p className="mt-1 text-xs text-content-muted">
                    El texto se compara contra la <b className="text-content">fila de títulos</b> (la{" "}
                    {filaEncabezado + 1}), no contra los datos de abajo. Se completa solo al elegir
                    la columna.
                  </p>
                )}
              </div>
            </div>

            {/* PASO 5 — probar ANTES de guardar. Es lo único que convierte números en certeza. */}
            <div className="flex flex-wrap items-center gap-2">
              <Button variant="secondary" onClick={probarAhora} loading={probar.isPending} disabled={!archivo}>
                Probar con el archivo
              </Button>
              <Button onClick={guardarAhora} loading={guardar.isPending} disabled={!puedeGuardar}>
                {b.id ? "Guardar cambios" : "Guardar el formato"}
              </Button>
              <Button variant="ghost" onClick={limpiar}>Cancelar</Button>
              {faltanColumnas.length > 0 && (
                <span className="text-xs text-pendiente">
                  Falta elegir: {faltanColumnas.map((p) => p.label).join(", ")}.
                </span>
              )}
            </div>

            {prueba && <ResultadoPrueba p={prueba} />}
          </CardContent>
        </Card>
      )}

      {listaQ.isPending ? (
        <LoadingState label="Cargando formatos" />
      ) : listaQ.isError ? (
        <ErrorState message={mensajeError(listaQ.error)} onRetry={() => listaQ.refetch()} />
      ) : formatos.length === 0 ? (
        <EmptyState message="No hay formatos propios configurados. Los seis bancos de arriba se reconocen igual." />
      ) : (
        <div className="flex flex-col gap-2">
          {formatos.map((f) => (
            <Card key={f.id}>
              <CardContent className="flex flex-wrap items-center gap-3 pt-5">
                <span className="font-medium text-content">{f.nombre}</span>
                {f.activo ? <Badge tone="positivo">activo</Badge> : <Badge tone="neutral">apagado</Badge>}
                <span className="text-xs text-content-muted">
                  fecha {f.formato_fecha} · {f.senales.length} señal(es)
                </span>
                {puedeEditar && (
                  <div className="ml-auto flex gap-2">
                    <Button size="sm" variant="secondary" onClick={() => editar(f)}>Editar</Button>
                    <Button
                      size="sm"
                      variant="ghost"
                      loading={borrar.isPending}
                      onClick={() => {
                        borrar.mutate(f.id, {
                          onSuccess: () => toast.success(`Formato «${f.nombre}» eliminado.`),
                          onError: (e) => toast.error(mensajeError(e)),
                        });
                      }}
                    >
                      Eliminar
                    </Button>
                  </div>
                )}
              </CardContent>
            </Card>
          ))}
        </div>
      )}
    </div>
  );
}

/** «3 — Haber» cuando se conoce el encabezado; solo el número cuando no. */
function etiquetaColumna(i: number, encabezado?: string[]): string {
  const texto = tituloDe(i, encabezado);
  return texto ? `${i} — ${texto}` : String(i);
}

/** El título de esa columna en la fila marcada como encabezado, o "" si no se marcó ninguna. */
function tituloDe(i: number, encabezado?: string[]): string {
  return (encabezado?.[i] ?? "").trim();
}

/**
 * Lo que el sistema ENTENDIÓ. Es la única parte de esta pantalla que convierte números de columna
 * en certeza: si el depósito aparece como débito, se ve acá y no tres semanas después.
 */
function ResultadoPrueba({ p }: { p: PruebaDeFormato }) {
  if (!p.detectado) {
    const fallan = p.senales_que_fallan ?? [];
    return (
      <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 p-3 text-sm">
        <p className="font-medium text-pendiente">No se reconoció el archivo con estas señales.</p>
        {/* CUÁL falla, no «alguna». Las señales se unen con Y: con cinco configuradas, saber que
            «no se reconoció» no dice por dónde empezar, y las cinco se ven igual de plausibles. */}
        {fallan.length > 0 ? (
          <>
            <p className="mt-2 text-xs text-content">
              {fallan.length === 1 ? "Esta señal no se cumple" : `Estas ${fallan.length} señales no se cumplen`}
              {` (comparado contra la fila ${fallan[0]!.fila_titulos}, la de los títulos):`}
            </p>
            <ul className="mt-1 flex flex-col gap-1">
              {fallan.map((s, i) => (
                <li key={i} className="text-xs text-content">
                  Columna <b>{s.columna}</b>: pedís que diga «<b>{s.pedido}</b>», y ahí dice{" "}
                  {s.encontrado ? <>«<b>{s.encontrado}</b>»</> : <i>(nada)</i>}.
                </li>
              ))}
            </ul>
            <p className="mt-2 text-xs text-content-muted">
              Volvé a elegir la columna en esa señal y el texto se completa solo.
            </p>
          </>
        ) : (
          <p className="mt-0.5 text-xs text-content-muted">
            No se encontró ninguna fila que parezca la de títulos. Revisá que sea la hoja correcta
            del Excel y que la fila marcada arriba sea la de los nombres de las columnas.
          </p>
        )}
      </div>
    );
  }
  return (
    <div className="rounded-lg border border-positivo/40 bg-positivo/5 p-3">
      <p className="text-sm font-medium text-positivo">
        Reconocido · {p.movimientos} movimiento(s){p.iban ? ` · cuenta ${p.iban}` : ""}
      </p>
      {p.problema && <p className="mt-1 text-xs text-pendiente">{p.problema}</p>}
      {p.muestra.length > 0 && (
        <table className="mt-2 w-full text-xs">
          <thead>
            <tr className="text-content-muted">
              <th className="px-2 py-1 text-left">Fecha</th>
              <th className="px-2 py-1 text-left">Referencia</th>
              <th className="px-2 py-1 text-left">Descripción</th>
              <th className="px-2 py-1 text-right">Débito</th>
              <th className="px-2 py-1 text-right">Crédito</th>
            </tr>
          </thead>
          <tbody>
            {p.muestra.map((m, i) => (
              <tr key={i} className="text-content">
                <td className="px-2 py-1 tabular-nums">{m.fecha}</td>
                <td className="px-2 py-1 font-mono">{m.documento || "—"}</td>
                <td className="max-w-60 truncate px-2 py-1">{m.descripcion}</td>
                <td className="px-2 py-1 text-right tabular-nums">{m.debito}</td>
                <td className="px-2 py-1 text-right tabular-nums">{m.credito}</td>
              </tr>
            ))}
          </tbody>
        </table>
      )}
      <p className="mt-2 text-xs text-content-muted">
        Mirá que lo que ENTRÓ esté en crédito y lo que SALIÓ en débito. Si están cambiados, el
        archivo se leería al revés y el cuadre del mes no cerraría.
      </p>
    </div>
  );
}
