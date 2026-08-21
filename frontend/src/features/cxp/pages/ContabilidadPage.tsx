/**
 * CxP — Facturas de Contabilidad (/cxp/contabilidad).
 *
 * El cuadro de las excepciones vigentes. Marcar un proveedor o un rubro hace que sus facturas no
 * requieran validación de área, y esa decisión tiene que poder verse ENTERA en una pantalla: si la
 * única forma de saber qué está marcado es abrir proveedor por proveedor, la excepción se vuelve
 * invisible y nadie la audita.
 *
 * Se elige escribiendo (BuscadorMultiple) porque hay cientos de clasificaciones: una parrilla de
 * chips con 168 opciones —y van a ser más— no se lee.
 */

import { useMemo, useState } from "react";
import {
  Badge,
  BuscadorMultiple,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  LoadingState,
  PageHeader,
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useMarcarClasificacionContabilidad,
  useMarcarConceptoContabilidad,
  useMarcarProveedorContabilidad,
  useMarcasContabilidad,
  useTodosProveedores,
} from "@/features/cxp/hooks";
import { useClasificaciones, useConceptos } from "@/features/bancos/hooks";
import type { MarcaContabilidad } from "@/api/cxp";

export function ContabilidadPage() {
  const toast = useToast();
  const tiene = useTienePermiso();
  const puedeMarcar = tiene("cxp.marcar_contabilidad");

  const marcasQ = useMarcasContabilidad();
  const proveedoresQ = useTodosProveedores();
  const conceptosQ = useConceptos("cxp");
  const clasifQ = useClasificaciones("cxp");

  const marcarProv = useMarcarProveedorContabilidad();
  const marcarCon = useMarcarConceptoContabilidad();
  const marcarCla = useMarcarClasificacionContabilidad();

  // Selección para AGREGAR. Lo ya marcado se quita desde su propia ficha, no desde acá: mezclar
  // agregar y quitar en el mismo control hace que un clic distraído desmarque algo vigente.
  const [nuevosProv, setNuevosProv] = useState<string[]>([]);
  const [nuevosCon, setNuevosCon] = useState<string[]>([]);
  const [nuevasCla, setNuevasCla] = useState<string[]>([]);

  const marcas = marcasQ.data;
  const yaProv = new Set((marcas?.proveedores ?? []).map((m) => m.id));
  const yaCon = new Set((marcas?.conceptos ?? []).map((m) => m.id));
  const yaCla = new Set((marcas?.clasificaciones ?? []).map((m) => m.id));

  const opcProv = useMemo(
    () => (proveedoresQ.data ?? []).filter((p) => !yaProv.has(p.id)).map((p) => ({ value: p.id, label: p.nombre })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [proveedoresQ.data, marcas],
  );
  const opcCon = useMemo(
    () => (conceptosQ.data ?? []).filter((c) => !yaCon.has(c.id)).map((c) => ({ value: c.id, label: c.nombre })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [conceptosQ.data, marcas],
  );
  const opcCla = useMemo(
    () =>
      (clasifQ.data ?? [])
        .filter((c) => !yaCla.has(c.id))
        .map((c) => ({ value: c.id, label: c.nombre, grupo: c.concepto })),
    // eslint-disable-next-line react-hooks/exhaustive-deps
    [clasifQ.data, marcas],
  );

  const guardando = marcarProv.isPending || marcarCon.isPending || marcarCla.isPending;

  /** Aplica el mismo valor a varios ids, uno por uno, y reporta el resultado agregado. */
  async function aplicar(
    ids: string[],
    valor: boolean,
    mutar: (v: { id: string; valor: boolean }) => Promise<unknown>,
    limpiar: () => void,
    queEs: string,
  ) {
    if (ids.length === 0) return;
    let ok = 0;
    const fallos: string[] = [];
    for (const id of ids) {
      try {
        await mutar({ id, valor });
        ok++;
      } catch (err) {
        fallos.push(mensajeError(err));
      }
    }
    limpiar();
    if (ok > 0) {
      toast.success(
        valor
          ? `${ok} ${queEs} marcado(s): sus facturas abiertas ya no esperan validación de área.`
          : `${ok} ${queEs} desmarcado(s): sus facturas vuelven a requerir validación de área.`,
      );
    }
    // Los fallos se dicen: un lote donde algo no se aplicó y nadie avisa es peor que un error.
    if (fallos.length > 0) toast.error(`${fallos.length} no se pudo(ieron) aplicar: ${fallos[0]}`);
  }

  if (marcasQ.isPending) return <LoadingState label="Cargando marcas" />;
  if (marcasQ.isError) return <ErrorState message={mensajeError(marcasQ.error)} onRetry={() => marcasQ.refetch()} />;

  const total =
    (marcas?.proveedores.length ?? 0) + (marcas?.conceptos.length ?? 0) + (marcas?.clasificaciones.length ?? 0);

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Facturas de Contabilidad"
        description="El gasto que ningún área puede validar: honorarios contables, timbres, comisiones bancarias, Hacienda, auditoría. Lo marcado acá no espera validación de área — la aprobación por monto se aplica igual."
      />

      <div className="rounded-xl border border-border bg-surface-muted px-4 py-3 text-sm">
        {total === 0 ? (
          <>
            Todavía no hay nada marcado. Marcá el <b>proveedor</b> cuando sus facturas <i>siempre</i> son
            de Contabilidad (el contador, el banco, Hacienda), o el <b>rubro</b> cuando lo que define el
            caso es el tipo de gasto. Una factura suelta se marca desde su propio expediente.
          </>
        ) : (
          <>
            <b>{total}</b> excepción{total === 1 ? "" : "es"} vigente{total === 1 ? "" : "s"}. Cada una hace
            que las facturas que le correspondan se aprueben <b>sin validación de área</b>.
          </>
        )}
      </div>

      <BloqueMarca
        titulo="Proveedores"
        ayuda="Es la marca que captura el «siempre»: se pone una vez y las facturas siguientes nacen así. Es retroactiva sobre las que todavía no se aprobaron — justamente las que estaban trancadas."
        marcados={marcas?.proveedores ?? []}
        opciones={opcProv}
        seleccion={nuevosProv}
        onSeleccion={setNuevosProv}
        etiquetaBuscador="Agregar proveedores"
        puedeMarcar={puedeMarcar}
        guardando={guardando}
        onAgregar={() =>
          void aplicar(nuevosProv, true, (v) => marcarProv.mutateAsync(v), () => setNuevosProv([]), "proveedor(es)")
        }
        onQuitar={(m) =>
          void aplicar([m.id], false, (v) => marcarProv.mutateAsync(v), () => {}, "proveedor(es)")
        }
      />

      <BloqueMarca
        titulo="Conceptos (rubros completos)"
        ayuda="Marca TODO el rubro. Es la más amplia de las tres: úsala solo cuando el concepto entero es de Contabilidad (p. ej. «Impuestos»). Si el rubro mezcla gasto de áreas, marcá la clasificación."
        marcados={marcas?.conceptos ?? []}
        opciones={opcCon}
        seleccion={nuevosCon}
        onSeleccion={setNuevosCon}
        etiquetaBuscador="Agregar conceptos"
        puedeMarcar={puedeMarcar}
        guardando={guardando}
        onAgregar={() =>
          void aplicar(nuevosCon, true, (v) => marcarCon.mutateAsync(v), () => setNuevosCon([]), "concepto(s)")
        }
        onQuitar={(m) => void aplicar([m.id], false, (v) => marcarCon.mutateAsync(v), () => {}, "concepto(s)")}
      />

      <BloqueMarca
        titulo="Clasificaciones"
        ayuda="El nivel fino: «Gastos › Comisiones bancarias» sin arrastrar el resto de «Gastos». Es la opción preferible cuando el concepto es amplio."
        marcados={marcas?.clasificaciones ?? []}
        opciones={opcCla}
        seleccion={nuevasCla}
        onSeleccion={setNuevasCla}
        etiquetaBuscador="Agregar clasificaciones"
        puedeMarcar={puedeMarcar}
        guardando={guardando}
        onAgregar={() =>
          void aplicar(nuevasCla, true, (v) => marcarCla.mutateAsync(v), () => setNuevasCla([]), "clasificación(es)")
        }
        onQuitar={(m) => void aplicar([m.id], false, (v) => marcarCla.mutateAsync(v), () => {}, "clasificación(es)")}
      />

      {!puedeMarcar && (
        <p className="text-xs text-content-muted">
          Solo lectura: no tenés el permiso para cambiar estas marcas.
        </p>
      )}
    </div>
  );
}

function BloqueMarca(props: {
  titulo: string;
  ayuda: string;
  marcados: MarcaContabilidad[];
  opciones: { value: string; label: string; grupo?: string }[];
  seleccion: string[];
  onSeleccion: (v: string[]) => void;
  etiquetaBuscador: string;
  puedeMarcar: boolean;
  guardando: boolean;
  onAgregar: () => void;
  onQuitar: (m: MarcaContabilidad) => void;
}) {
  return (
    <Card>
      <CardHeader>
        <CardTitle>
          {props.titulo}{" "}
          <span className="text-sm font-normal text-content-muted">· {props.marcados.length} marcado(s)</span>
        </CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        <p className="text-sm text-content-muted">{props.ayuda}</p>

        {props.marcados.length > 0 ? (
          <div className="flex flex-wrap gap-1.5">
            {props.marcados.map((m) => (
              <span
                key={m.id}
                title={
                  m.activo
                    ? undefined
                    : "Está desactivado, pero la marca sigue vigente para sus facturas abiertas"
                }
                className={
                  m.activo
                    ? "inline-flex items-center gap-1.5 rounded-lg border border-accent bg-accent/10 px-2 py-1 text-xs font-medium text-accent"
                    : "inline-flex items-center gap-1.5 rounded-lg border border-pendiente bg-pendiente/10 px-2 py-1 text-xs font-medium text-content-muted"
                }
              >
                {m.concepto && <span className="font-normal opacity-70">{m.concepto} ›</span>}
                {m.nombre}
                {/* Desactivar NO quita la marca: la excepción sigue viva y hay que verla. */}
                {!m.activo && <span className="font-normal italic opacity-80">(inactivo)</span>}
                {props.puedeMarcar && (
                  <button
                    type="button"
                    aria-label={`Quitar ${m.nombre}`}
                    className="rounded px-0.5 leading-none hover:bg-accent/20"
                    onClick={() => props.onQuitar(m)}
                    disabled={props.guardando}
                  >
                    ✕
                  </button>
                )}
              </span>
            ))}
          </div>
        ) : (
          <Badge tone="neutral">Ninguno marcado</Badge>
        )}

        {props.puedeMarcar && (
          <div className="flex flex-col gap-2 border-t border-border pt-4">
            <BuscadorMultiple
              label={props.etiquetaBuscador}
              leyendaVacio="ninguno elegido"
              placeholder="Escribí para buscar…"
              opciones={props.opciones}
              seleccion={props.seleccion}
              onChange={props.onSeleccion}
            />
            {props.seleccion.length > 0 && (
              <div>
                <Button onClick={props.onAgregar} loading={props.guardando}>
                  Marcar {props.seleccion.length} como de Contabilidad
                </Button>
              </div>
            )}
          </div>
        )}
      </CardContent>
    </Card>
  );
}
