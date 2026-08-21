/**
 * Diccionario del catálogo: exportar e importar Concepto › Clasificación con palabras clave.
 *
 * Lo que resuelve: armar el catálogo en Excel (donde la gente trabaja), llevárselo de una
 * empresa a otra, y que una fila con palabras clave se vuelva REGLA del motor al importarla —
 * la regla se aplica de una vez a lo que está sin clasificar.
 *
 * Nunca se aplica a ciegas: primero se previsualiza el plan y después se confirma. El plan que
 * se ve en pantalla es el mismo que ejecuta el servidor.
 */

import { useRef, useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { descargarBlob } from "@/lib/csv";
import { mensajeError } from "@/lib/apiError";
import { bancosApi, type PlanDiccionario } from "@/api/bancos";
import { useImportarDiccionario } from "@/features/bancos/hooks";
import { useTienePermiso } from "@/features/auth/permisos";

export function DiccionarioPanel() {
  const toast = useToast();
  const tienePermiso = useTienePermiso();
  const puedeImportar = tienePermiso("bancos.reglas");
  const puedeExportar = tienePermiso("bancos.exportar");

  const inputRef = useRef<HTMLInputElement>(null);
  const [archivo, setArchivo] = useState<File | null>(null);
  const [plan, setPlan] = useState<PlanDiccionario | null>(null);
  const [descargando, setDescargando] = useState(false);
  const importar = useImportarDiccionario();

  async function exportar() {
    setDescargando(true);
    try {
      const blob = await bancosApi.exportarDiccionario();
      descargarBlob("diccionario-catalogo.xlsx", blob);
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setDescargando(false);
    }
  }

  function elegir(f: File | null) {
    setArchivo(f);
    setPlan(null);
    if (!f) return;
    importar.mutate(
      { archivo: f, aplicar: false },
      {
        onSuccess: setPlan,
        onError: (err) => {
          toast.error(mensajeError(err));
          setArchivo(null);
        },
      },
    );
  }

  function aplicar() {
    if (!archivo) return;
    importar.mutate(
      { archivo, aplicar: true },
      {
        onSuccess: (p) => {
          setPlan(p);
          const partes = [
            p.conceptos_nuevos && `${p.conceptos_nuevos} concepto(s)`,
            p.clasificaciones_nuevas && `${p.clasificaciones_nuevas} clasificación(es)`,
            p.reglas_nuevas && `${p.reglas_nuevas} regla(s)`,
          ].filter(Boolean);
          toast.success(
            partes.length === 0
              ? "El catálogo ya estaba completo: no hubo cambios."
              : `Se creó ${partes.join(", ")}.` +
                  (p.clasificados > 0
                    ? ` Las reglas clasificaron ${p.clasificados.toLocaleString("es-CR")} movimientos.`
                    : ""),
          );
          setArchivo(null);
          if (inputRef.current) inputRef.current.value = "";
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const aplicable = plan
    ? plan.conceptos_nuevos +
      plan.clasificaciones_nuevas +
      plan.reglas_nuevas +
      plan.naturalezas_declaradas
    : 0;

  return (
    <Card>
      <CardHeader>
        <CardTitle>Diccionario del catálogo</CardTitle>
        <p className="mt-0.5 text-xs text-content-muted">
          Bajá el catálogo a Excel, editalo y volvelo a subir. Si una fila trae{" "}
          <b className="text-content">palabras clave</b>, el concepto se vuelve regla del motor y se
          aplica a lo que está sin clasificar. Importar nunca renombra ni borra: solo agrega lo que falta.
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <div className="flex flex-wrap items-center gap-3">
          {puedeExportar && (
            <Button variant="secondary" onClick={exportar} loading={descargando}>
              Exportar a Excel
            </Button>
          )}
          {puedeImportar && (
            <>
              <input
                ref={inputRef}
                type="file"
                accept=".xlsx,.xlsm"
                onChange={(e) => elegir(e.target.files?.[0] ?? null)}
                className="text-sm text-content-muted file:mr-3 file:cursor-pointer file:rounded-lg file:border file:border-border file:bg-surface-muted file:px-3 file:py-2 file:text-sm file:text-content hover:file:bg-surface-raised"
                aria-label="Archivo del diccionario"
              />
              {importar.isPending && !plan && (
                <span className="text-xs text-content-muted">Leyendo el archivo…</span>
              )}
            </>
          )}
        </div>

        <p className="text-xs text-content-muted">
          Columnas: <span className="font-mono">Concepto</span> ·{" "}
          <span className="font-mono">Clasificación</span> ·{" "}
          <span className="font-mono">Visible en CxP</span> ·{" "}
          <span className="font-mono">Naturaleza</span> (ingreso/gasto/no entra) ·{" "}
          <span className="font-mono">Palabras clave</span> (separadas por{" "}
          <span className="font-mono">;</span>) · <span className="font-mono">Aplica a</span>{" "}
          (débito/crédito/mixto) · <span className="font-mono">Prioridad</span>. El archivo que exportás
          se puede volver a importar tal cual.
        </p>

        <p className="text-xs text-content-muted">
          La <span className="font-mono">Naturaleza</span> es la forma rápida de declarar en Excel qué
          entra al EBITDA y qué no, en vez de concepto por concepto en la pantalla. Solo se declara lo
          que <b className="text-content">nadie declaró todavía</b>: si el archivo dice algo distinto
          de lo ya decidido, se avisa y no se cambia —cambiar la naturaleza mueve el resultado de
          todos los meses—. Una celda vacía no dice «no entra»: dice «esta fila no opina».
        </p>

        {plan && <PlanVista plan={plan} />}

        {plan && !plan.aplicado && (
          <div className="flex flex-wrap items-center gap-3 border-t border-border pt-3">
            <Button onClick={aplicar} loading={importar.isPending} disabled={aplicable === 0}>
              {aplicable === 0 ? "No hay nada que crear" : `Aplicar (${aplicable} cambios)`}
            </Button>
            <Button
              variant="ghost"
              onClick={() => {
                setPlan(null);
                setArchivo(null);
                if (inputRef.current) inputRef.current.value = "";
              }}
            >
              Descartar
            </Button>
          </div>
        )}
      </CardContent>
    </Card>
  );
}

function PlanVista({ plan }: { plan: PlanDiccionario }) {
  const conAccion = plan.acciones.filter(
    (a) => a.crear_concepto || a.crear_clasificacion || a.crear_regla || a.problema,
  );
  return (
    <div className="flex flex-col gap-3">
      <div className="flex flex-wrap gap-x-6 gap-y-2 rounded-lg border border-border bg-surface-muted px-4 py-3 text-sm">
        <Cifra titulo="Filas leídas" valor={plan.filas} />
        <Cifra titulo="Conceptos nuevos" valor={plan.conceptos_nuevos} tono="accent" />
        <Cifra titulo="Clasificaciones nuevas" valor={plan.clasificaciones_nuevas} tono="accent" />
        <Cifra titulo="Reglas nuevas" valor={plan.reglas_nuevas} tono="accent" />
        {plan.naturalezas_declaradas > 0 && (
          <Cifra titulo="Naturalezas declaradas" valor={plan.naturalezas_declaradas} tono="accent" />
        )}
        {plan.naturalezas_en_conflicto > 0 && (
          <Cifra
            titulo="Naturalezas que no se cambian"
            valor={plan.naturalezas_en_conflicto}
            tono="pendiente"
          />
        )}
        <Cifra titulo="Sin cambios" valor={plan.sin_cambios} />
        <Cifra titulo="Omitidas" valor={plan.omitidas} tono={plan.omitidas > 0 ? "pendiente" : undefined} />
        {plan.aplicado && plan.clasificados > 0 && (
          <Cifra titulo="Movimientos clasificados" valor={plan.clasificados} tono="positivo" />
        )}
      </div>

      {conAccion.length > 0 && (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH className="text-right">Línea</TH>
                <TH>Concepto › Clasificación</TH>
                <TH>Palabras clave</TH>
                <TH>{plan.aplicado ? "Resultado" : "Se hará"}</TH>
              </TR>
            </THead>
            <TBody>
              {conAccion.map((a) => (
                <TR key={a.linea}>
                  <TD className="text-right tabular-nums text-content-muted">{a.linea}</TD>
                  <TD>
                    <span className="font-medium">{a.concepto || "—"}</span>
                    {a.clasificacion && <span className="text-content-muted"> › {a.clasificacion}</span>}
                  </TD>
                  <TD className="font-mono text-xs text-content-muted">{a.palabras || "—"}</TD>
                  <TD>
                    {a.problema ? (
                      <span className="text-xs text-pendiente">{a.problema}</span>
                    ) : (
                      <span className="flex flex-wrap gap-1">
                        {a.crear_concepto && <Badge tone="accent">concepto</Badge>}
                        {a.crear_clasificacion && <Badge tone="accent">clasificación</Badge>}
                        {a.crear_regla && (
                          <Badge tone="positivo">regla {a.aplica_a.toLowerCase()}</Badge>
                        )}
                      </span>
                    )}
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}
    </div>
  );
}

function Cifra({
  titulo,
  valor,
  tono,
}: {
  titulo: string;
  valor: number;
  tono?: "accent" | "positivo" | "pendiente";
}) {
  return (
    <div>
      <p className="text-xs uppercase tracking-wide text-content-muted">{titulo}</p>
      <p
        className={cn(
          "text-lg font-semibold tabular-nums",
          tono === "accent" && "text-accent",
          tono === "positivo" && "text-positivo",
          tono === "pendiente" && "text-pendiente",
        )}
      >
        {valor.toLocaleString("es-CR")}
      </p>
    </div>
  );
}
