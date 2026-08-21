/**
 * «Traer la clasificación desde Excel» — el panel que aprovecha el trabajo ya hecho a mano.
 *
 * Existe porque el usuario tiene los bancos de 2025 y 2026 en un Excel con la mitad ya clasificada
 * (2026-08-18), y ese trabajo no se podía traer: el importador de estados de cuenta carga todo como
 * «No identificado» y no lee ninguna columna de partida.
 *
 * El flujo es el que ya usa el diccionario del catálogo: se baja una plantilla, se llena en Excel, se
 * sube, se VE qué va a pasar con cada fila y solo entonces se aplica.
 *
 * Dos guardarraíles que se explican en pantalla porque cambian lo que ocurre:
 *  · Esto CLASIFICA movimientos ya cargados; no los crea. Una fila cuyo movimiento no esté importado
 *    lo dice («no está cargado»), no se salta en silencio.
 *  · Por defecto NO toca lo que ya tiene partida. Pisar miles de decisiones por subir un archivo es
 *    lo que no debe poder pasar sin pedirlo, así que reemplazar es una casilla aparte.
 */

import { useRef, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  Select,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
} from "@/components/ui";
import type { BadgeTone } from "@/components/ui";
import { bancosApi } from "@/api/bancos";
import type { CuentaBancaria, EstadoClasifExcel, PlanClasifExcel } from "@/api/bancos";
import { descargarBlob } from "@/lib/csv";
import { mensajeError } from "@/lib/apiError";
import { hoyCR } from "@/lib/format";
import { useClasificarDesdeExcel } from "@/features/bancos/hooks";
import { useTienePermiso } from "@/features/auth/permisos";

/** Tono y etiqueta de cada estado. La etiqueta dice la CONSECUENCIA, no el nombre técnico. */
const TONO: Record<EstadoClasifExcel, BadgeTone> = {
  CLASIFICA: "positivo",
  RECLASIFICA: "pendiente",
  SIN_CAMBIO: "accent",
  SIN_MOVIMIENTO: "negativo",
  PARTIDA_DESCONOCIDA: "negativo",
  CUENTA_DESCONOCIDA: "negativo",
  FILA_INVALIDA: "negativo",
  AMBIGUO: "pendiente",
  YA_CLASIFICADO: "pendiente",
  SIN_LLENAR: "neutral",
};

const ETIQUETA: Record<EstadoClasifExcel, string> = {
  CLASIFICA: "se clasifica",
  RECLASIFICA: "se reemplaza",
  SIN_CAMBIO: "ya la tenía",
  SIN_MOVIMIENTO: "no está cargado",
  PARTIDA_DESCONOCIDA: "partida que no existe",
  CUENTA_DESCONOCIDA: "cuenta que no existe",
  FILA_INVALIDA: "no se pudo leer",
  AMBIGUO: "no se sabe cuál",
  YA_CLASIFICADO: "ya tiene otra",
  SIN_LLENAR: "sin llenar",
};

/** Un año hacia atrás desde hoy: el rango con el que se baja la plantilla por defecto. */
function haceUnAnio(): string {
  const hoy = hoyCR();
  const [a, m, d] = hoy.split("-");
  return `${Number(a) - 1}-${m}-${d}`;
}

export function ClasificarDesdeExcelPanel({ cuentas }: { cuentas: CuentaBancaria[] }) {
  // Mismo criterio que el panel gemelo del diccionario: si el rol no puede hacerlo, no se muestra.
  // El backend ya lo exige (bancos.clasificar / bancos.exportar); acá se evita ofrecer un botón que
  // termina en un 403.
  const tienePermiso = useTienePermiso();
  const puedeClasificar = tienePermiso("bancos.clasificar");
  const puedeExportar = tienePermiso("bancos.exportar");
  const toast = useToast();
  const importar = useClasificarDesdeExcel();
  const inputRef = useRef<HTMLInputElement>(null);

  const [desde, setDesde] = useState(haceUnAnio);
  const [hasta, setHasta] = useState(hoyCR);
  const [archivo, setArchivo] = useState<File | null>(null);
  const [cuentaId, setCuentaId] = useState("");
  const [reemplazar, setReemplazar] = useState(false);
  const [plan, setPlan] = useState<PlanClasifExcel | null>(null);
  const [descargando, setDescargando] = useState(false);

  async function bajarPlantilla() {
    setDescargando(true);
    try {
      const { blob, filename } = await bancosApi.plantillaClasificacion(desde, hasta, true);
      descargarBlob(filename || "clasificar.xlsx", blob);
      toast.success(
        "Plantilla descargada. Llená las columnas Concepto y Clasificación y volvé a subirla.",
      );
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setDescargando(false);
    }
  }

  /**
   * Pide la previsualización con las opciones EXACTAS que se van a aplicar.
   *
   * Recibe las opciones por parámetro en vez de leerlas del estado porque React agrupa las
   * actualizaciones: al marcar «reemplazar» y previsualizar en el mismo evento, el estado todavía
   * tiene el valor viejo y la pantalla mostraría un plan que no corresponde a lo que Aplicar haría.
   */
  function previsualizar(f: File, opts: { cuentaId: string; reemplazar: boolean }) {
    importar.mutate(
      {
        archivo: f,
        cuentaBancariaId: opts.cuentaId || undefined,
        reemplazar: opts.reemplazar,
        aplicar: false,
      },
      {
        onSuccess: (p) => setPlan(p),
        onError: (err) => {
          toast.error(mensajeError(err));
          setPlan(null);
        },
      },
    );
  }

  function elegir(f: File | null) {
    setPlan(null);
    if (!f) {
      setArchivo(null);
      return;
    }
    if (!f.name.toLowerCase().endsWith(".xlsx")) {
      toast.error("El archivo debe ser un Excel .xlsx");
      if (inputRef.current) inputRef.current.value = "";
      return;
    }
    setArchivo(f);
    // Elegir el archivo ya previsualiza: un paso menos, y nada se escribe.
    previsualizar(f, { cuentaId, reemplazar });
  }

  /** Cambiar una opción no puede dejar en pantalla un plan que ya no corresponde: se vuelve a pedir. */
  function cambiarOpcion(cambio: { cuentaId?: string; reemplazar?: boolean }) {
    const nuevaCuenta = cambio.cuentaId ?? cuentaId;
    const nuevoReemplazar = cambio.reemplazar ?? reemplazar;
    if (cambio.cuentaId !== undefined) setCuentaId(cambio.cuentaId);
    if (cambio.reemplazar !== undefined) setReemplazar(cambio.reemplazar);
    if (!archivo) return;
    setPlan(null);
    previsualizar(archivo, { cuentaId: nuevaCuenta, reemplazar: nuevoReemplazar });
  }

  function aplicar() {
    if (!archivo) return;
    importar.mutate(
      { archivo, cuentaBancariaId: cuentaId || undefined, reemplazar, aplicar: true },
      {
        onSuccess: (p) => {
          setPlan(p);
          // No se afirma la causa: el motivo real está en los contadores y en la tabla, y decir
          // «ya tenían su partida» sería falso cuando lo que pasó fue que no estaban cargados.
          if (p.clasificados === 0) {
            toast.info("No se clasificó ningún movimiento. Mirá el detalle para ver por qué.");
          } else {
            toast.success(
              `${p.clasificados.toLocaleString("es-CR")} movimiento(s) quedaron clasificados.`,
            );
          }
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const aplicables = plan ? plan.clasifica + plan.reclasifica : 0;

  if (!puedeClasificar && !puedeExportar) return null;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Traer la clasificación desde Excel</CardTitle>
        <p className="mt-1 text-xs text-content-muted">
          Si ya clasificaste movimientos en una hoja de Excel, no hay que volver a hacerlo acá. Bajá
          la plantilla, pegá o escribí el <strong>Concepto</strong> y la{" "}
          <strong>Clasificación</strong> de cada línea y subila. Esto <strong>clasifica</strong>{" "}
          movimientos ya cargados: no los crea. Si un movimiento todavía no está importado, la fila
          lo dice y se importa primero el estado de cuenta de ese mes.
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/* Paso 1 — bajar la plantilla del rango */}
        {puedeExportar && (
        <div className="flex flex-col gap-2 rounded-lg border border-border bg-surface-muted p-3">
          <p className="text-sm font-medium text-content">1. Bajá la plantilla</p>
          <p className="text-xs text-content-muted">
            Trae los movimientos <strong>sin clasificar</strong> del rango, con las dos columnas de
            partida vacías. Las columnas se reconocen por su nombre, así que podés reordenarlas o
            usar tu propio archivo si tiene Fecha, Documento, Débito, Crédito y Clasificación.
          </p>
          <div className="flex flex-wrap items-end gap-2">
            <label className="flex flex-col gap-1 text-xs text-content-muted">
              Desde
              <input
                type="date"
                value={desde}
                onChange={(e) => setDesde(e.target.value)}
                className="rounded-md border border-border bg-surface px-2 py-1.5 text-sm text-content focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              />
            </label>
            <label className="flex flex-col gap-1 text-xs text-content-muted">
              Hasta
              <input
                type="date"
                value={hasta}
                onChange={(e) => setHasta(e.target.value)}
                className="rounded-md border border-border bg-surface px-2 py-1.5 text-sm text-content focus-visible:outline-none focus-visible:ring-2 focus-visible:ring-accent"
              />
            </label>
            <Button variant="secondary" onClick={bajarPlantilla} loading={descargando}>
              Bajar plantilla
            </Button>
          </div>
        </div>
        )}

        {/* Paso 2 — subir el archivo lleno */}
        {puedeClasificar && (
        <div className="flex flex-col gap-2 rounded-lg border border-border bg-surface-muted p-3">
          <p className="text-sm font-medium text-content">2. Subí el archivo lleno</p>
          <div className="flex flex-wrap items-end gap-3">
            <input
              ref={inputRef}
              type="file"
              accept=".xlsx"
              aria-label="Archivo con las clasificaciones"
              onChange={(e) => elegir(e.target.files?.[0] ?? null)}
              className="text-sm text-content-muted file:mr-3 file:cursor-pointer file:rounded-lg file:border file:border-border file:bg-surface file:px-3 file:py-2 file:text-sm file:text-content hover:file:bg-surface-raised"
            />
            <Select
              label="Si el archivo no dice la cuenta"
              value={cuentaId}
              onChange={(e) => cambiarOpcion({ cuentaId: e.target.value })}
              options={[
                { value: "", label: "— la trae el archivo —" },
                ...cuentas.map((c) => ({ value: c.id, label: `${c.banco} · ${c.alias}` })),
              ]}
              className="min-w-56"
            />
          </div>
          <label className="flex items-start gap-2 text-xs text-content-muted">
            <input
              type="checkbox"
              checked={reemplazar}
              onChange={(e) => cambiarOpcion({ reemplazar: e.target.checked })}
              className="mt-0.5"
            />
            <span>
              <strong className="text-content">Reemplazar lo ya clasificado.</strong> Sin esto, un
              movimiento que ya tiene partida no se toca (se muestra cuál tiene). Marcalo solo si el
              archivo es la versión correcta y querés que gane sobre lo que hay.
            </span>
          </label>
          {importar.isPending && !plan && (
            <span className="text-xs text-content-muted">Leyendo el archivo…</span>
          )}
        </div>
        )}

        {plan && <PlanVista plan={plan} />}

        {plan && !plan.aplicado && (
          <div className="flex flex-wrap items-center gap-3 border-t border-border pt-3">
            <Button onClick={aplicar} disabled={aplicables === 0} loading={importar.isPending}>
              {aplicables === 0
                ? "No hay nada que aplicar"
                : `Aplicar (${aplicables.toLocaleString("es-CR")} movimientos)`}
            </Button>
            <button
              type="button"
              className="text-xs text-accent underline"
              onClick={() => {
                setPlan(null);
                setArchivo(null);
                if (inputRef.current) inputRef.current.value = "";
              }}
            >
              Descartar
            </button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function PlanVista({ plan }: { plan: PlanClasifExcel }) {
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-x-6 gap-y-2 rounded-lg border border-border bg-surface-muted px-3 py-2">
        <Cifra rotulo="Filas leídas" valor={plan.filas} />
        <Cifra rotulo="Se clasifican" valor={plan.clasifica} tono="positivo" />
        {plan.reclasifica > 0 && (
          <Cifra rotulo="Se reemplazan" valor={plan.reclasifica} tono="pendiente" />
        )}
        {plan.sin_cambio > 0 && <Cifra rotulo="Ya la tenían" valor={plan.sin_cambio} />}
        {plan.protegidas > 0 && (
          <Cifra rotulo="Ya tienen otra" valor={plan.protegidas} tono="pendiente" />
        )}
        {plan.sin_movimiento > 0 && (
          <Cifra rotulo="Sin cargar" valor={plan.sin_movimiento} tono="negativo" />
        )}
        {plan.sin_partida > 0 && (
          <Cifra rotulo="Partida inexistente" valor={plan.sin_partida} tono="negativo" />
        )}
        {plan.sin_cuenta > 0 && (
          <Cifra rotulo="Sin cuenta" valor={plan.sin_cuenta} tono="negativo" />
        )}
        {plan.ambiguas > 0 && <Cifra rotulo="Ambiguas" valor={plan.ambiguas} tono="pendiente" />}
        {plan.invalidas > 0 && (
          <Cifra rotulo="Ilegibles" valor={plan.invalidas} tono="negativo" />
        )}
        {plan.sin_llenar > 0 && <Cifra rotulo="Sin llenar" valor={plan.sin_llenar} />}
        {plan.aplicado && (
          <Cifra rotulo="Clasificados" valor={plan.clasificados} tono="positivo" />
        )}
      </div>

      {plan.aviso && (
        <div className="rounded-lg border border-pendiente/40 bg-pendiente/10 px-3 py-2">
          <p className="text-xs text-content">{plan.aviso}</p>
        </div>
      )}

      {/* Qué hoja se leyó: se lee UNA sola y hay que poder verlo sin abrir el archivo. */}
      {plan.hoja && (
        <p className="text-xs text-content-muted">
          Se leyó la hoja <span className="font-mono">{plan.hoja}</span>
          {plan.hojas.length > 1 && ` de ${plan.hojas.length} que tiene el libro`}.
        </p>
      )}

      <TableContainer>
        <Table>
          <THead>
            <TR>
              <TH className="text-right">Línea</TH>
              <TH>Fecha</TH>
              <TH>Movimiento en el sistema</TH>
              <TH>Partida del archivo</TH>
              <TH className="text-right">Débito</TH>
              <TH className="text-right">Crédito</TH>
              <TH>{plan.aplicado ? "Resultado" : "Qué pasa"}</TH>
            </TR>
          </THead>
          <TBody>
            {plan.detalle.map((f) => (
              <TR key={`${f.linea}-${f.documento}-${f.fecha}`}>
                <TD className="text-right tabular-nums text-content-muted">{f.linea}</TD>
                <TD className="whitespace-nowrap tabular-nums">{f.fecha || "—"}</TD>
                <TD>
                  {/* La descripción es la del movimiento HALLADO: es la prueba de que calzó con el
                      correcto. Sin ella habría que creerle al sistema. */}
                  <span className="block text-xs text-content">
                    {f.descripcion || <span className="text-content-muted">(no se encontró)</span>}
                  </span>
                  <span className="block text-[11px] text-content-muted">
                    {f.cuenta || "—"} · doc {f.documento || "—"}
                  </span>
                </TD>
                <TD>
                  <span className="block text-xs text-content">{f.clasificacion || "—"}</span>
                  <span className="block text-[11px] text-content-muted">{f.concepto}</span>
                </TD>
                <TD className="text-right tabular-nums">{f.debito}</TD>
                <TD className="text-right tabular-nums">{f.credito}</TD>
                <TD>
                  <Badge tone={TONO[f.estado] ?? "pendiente"}>
                    {ETIQUETA[f.estado] ?? f.estado}
                  </Badge>
                  {f.detalle && (
                    <span className="mt-0.5 block text-[11px] text-content-muted">{f.detalle}</span>
                  )}
                </TD>
              </TR>
            ))}
          </TBody>
        </Table>
      </TableContainer>
      {plan.detalle_truncado && (
        <p className="text-xs text-content-muted">
          El archivo tiene {plan.filas.toLocaleString("es-CR")} filas y la tabla muestra{" "}
          {plan.detalle.length}: se priorizan las que necesitan atención. Los números de arriba son
          del total.
        </p>
      )}
    </div>
  );
}

function Cifra({
  rotulo,
  valor,
  tono,
}: {
  rotulo: string;
  valor: number;
  tono?: "accent" | "positivo" | "pendiente" | "negativo";
}) {
  const color =
    tono === "positivo"
      ? "text-positivo"
      : tono === "negativo"
        ? "text-negativo"
        : tono === "pendiente"
          ? "text-pendiente"
          : tono === "accent"
            ? "text-accent"
            : "text-content";
  return (
    <div className="flex flex-col">
      <span className="text-xs uppercase tracking-wide text-content-muted">{rotulo}</span>
      <span className={`text-lg font-semibold tabular-nums ${color}`}>
        {valor.toLocaleString("es-CR")}
      </span>
    </div>
  );
}
