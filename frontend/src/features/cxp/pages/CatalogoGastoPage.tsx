/**
 * CxP — Catálogo de gasto (/cxp/catalogo). La puerta de Contabilidad al catálogo.
 *
 * ── POR QUÉ EXISTE ──
 * Cuando entra una factura de un gasto que no está en el catálogo, no se puede clasificar y la
 * factura queda trancada. Medido el 3 de setiembre de 2026 en Valle de Paz: 834 de las 938 facturas
 * que esperaban validación de área estaban SIN CLASIFICAR, y solo 4 de los 22 rubros eran visibles
 * para CxP. Escribir el catálogo era exclusivo de `bancos.catalogo`, que abre además bancos,
 * cuentas, la naturaleza (el EBITDA) y la visibilidad.
 *
 * ── EL ALCANCE ES LA PANTALLA ──
 * Acá solo se abre y se renombra, y solo sobre los rubros visibles para Contabilidad. Apagar,
 * fusionar y declarar la naturaleza siguen en Bancos porque el catálogo es COMPARTIDO: tocan la
 * clasificación bancaria histórica y el resultado de la empresa. La pantalla lo dice en voz alta en
 * lugar de esconder los botones sin explicación.
 *
 * El listado se lee del catálogo de Bancos con ámbito «cxp» —es la misma tabla— y las mutaciones
 * van por `/cxp/catalogo/...` (permiso `cxp.catalogo`).
 */

import { useMemo, useState, type FormEvent } from "react";
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
  useToast,
} from "@/components/ui";
import { mensajeError } from "@/lib/apiError";
import { useClasificaciones, useConceptos } from "@/features/bancos/hooks";
import {
  useCrearClasificacionGasto,
  useCrearConceptoGasto,
  useRenombrarClasificacionGasto,
  useRenombrarConceptoGasto,
} from "@/features/cxp/hooks";
import type { ClasificacionCatalogo, ConceptoCatalogo } from "@/api/bancos";

export function CatalogoGastoPage() {
  const toast = useToast();
  const conceptosQ = useConceptos("cxp");
  const clasifsQ = useClasificaciones("cxp");
  const crearConcepto = useCrearConceptoGasto();

  const [nuevoRubro, setNuevoRubro] = useState("");
  const [mostrarForm, setMostrarForm] = useState(false);

  const conceptos = conceptosQ.data ?? [];
  const clasifs = clasifsQ.data ?? [];

  /** Clasificaciones agrupadas por rubro, en orden alfabético. */
  const porConcepto = useMemo(() => {
    const mapa = new Map<string, ClasificacionCatalogo[]>();
    for (const c of clasifs) {
      const lista = mapa.get(c.concepto_id);
      if (lista) lista.push(c);
      else mapa.set(c.concepto_id, [c]);
    }
    for (const lista of mapa.values()) lista.sort((a, b) => a.nombre.localeCompare(b.nombre));
    return mapa;
  }, [clasifs]);

  function onCrearRubro(e: FormEvent) {
    e.preventDefault();
    const nombre = nuevoRubro.trim();
    if (!nombre) return;
    crearConcepto.mutate(nombre, {
      onSuccess: () => {
        toast.success(`Rubro «${nombre}» abierto. Ya se puede usar para clasificar.`);
        setNuevoRubro("");
        setMostrarForm(false);
      },
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  const cargando = conceptosQ.isPending || clasifsQ.isPending;
  const error = conceptosQ.isError ? conceptosQ.error : clasifsQ.isError ? clasifsQ.error : null;

  return (
    <div className="flex flex-col gap-4">
      <PageHeader
        title="Catálogo de gasto"
        description="Los rubros con los que Contabilidad clasifica las facturas. Una factura sin rubro no se puede clasificar y queda trancada."
        actions={
          <Button
            variant={mostrarForm ? "secondary" : "primary"}
            onClick={() => {
              setMostrarForm((v) => !v);
              setNuevoRubro("");
            }}
          >
            {mostrarForm ? "Cerrar" : "+ Nuevo rubro"}
          </Button>
        }
      />

      <div className="flex items-start gap-2 rounded-lg border border-border bg-surface-raised px-3 py-2 text-sm text-content-muted">
        📚{" "}
        <span>
          Acá se ven <b className="text-content">solo los rubros de gasto</b>: el catálogo es
          compartido con Bancos, que además abre rubros de tesorería —traslados, ahorro, overnight—
          que no son gasto a pagar. Desde esta pantalla se <b className="text-content">abre</b> y se{" "}
          <b className="text-content">renombra</b>. Apagar, fusionar y declarar si el rubro es
          ingreso o gasto para el EBITDA quedan en el catálogo de Bancos: son decisiones que tocan la
          clasificación bancaria de años anteriores.
        </span>
      </div>

      {mostrarForm && (
        <Card>
          <CardHeader>
            <CardTitle>Nuevo rubro</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={onCrearRubro} className="flex flex-wrap items-end gap-2">
              <div className="min-w-64 flex-1">
                <Input
                  label="Nombre del rubro *"
                  value={nuevoRubro}
                  onChange={(e) => setNuevoRubro(e.target.value)}
                  placeholder="Ej. Servicios profesionales, Mantenimiento…"
                  hint="Nace visible para Contabilidad. Después se le cuelgan las clasificaciones."
                />
              </div>
              <Button type="submit" loading={crearConcepto.isPending} disabled={!nuevoRubro.trim()}>
                Abrir rubro
              </Button>
            </form>
          </CardContent>
        </Card>
      )}

      {cargando ? (
        <LoadingState label="Cargando el catálogo de gasto" />
      ) : error ? (
        <ErrorState
          message={mensajeError(error)}
          onRetry={() => {
            void conceptosQ.refetch();
            void clasifsQ.refetch();
          }}
        />
      ) : conceptos.length === 0 ? (
        <EmptyState message="Todavía no hay rubros de gasto visibles para Contabilidad. Abrí el primero con «+ Nuevo rubro»." />
      ) : (
        <div className="flex flex-col gap-3">
          {conceptos.map((con) => (
            <TarjetaRubro key={con.id} concepto={con} clasificaciones={porConcepto.get(con.id) ?? []} />
          ))}
        </div>
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Un rubro: su nombre (renombrable) y sus clasificaciones
// ---------------------------------------------------------------------------

function TarjetaRubro({
  concepto,
  clasificaciones,
}: {
  concepto: ConceptoCatalogo;
  clasificaciones: ClasificacionCatalogo[];
}) {
  const toast = useToast();
  const renombrarConcepto = useRenombrarConceptoGasto();
  const crearClasif = useCrearClasificacionGasto();

  const [editando, setEditando] = useState(false);
  const [nombre, setNombre] = useState(concepto.nombre);
  const [nueva, setNueva] = useState("");
  const [agregando, setAgregando] = useState(false);

  function guardarNombre(e: FormEvent) {
    e.preventDefault();
    const n = nombre.trim();
    if (!n) return;
    renombrarConcepto.mutate(
      { id: concepto.id, nombre: n },
      {
        onSuccess: () => {
          toast.success("Rubro renombrado.");
          setEditando(false);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  function agregarClasificacion(e: FormEvent) {
    e.preventDefault();
    const n = nueva.trim();
    if (!n) return;
    crearClasif.mutate(
      { conceptoId: concepto.id, nombre: n },
      {
        onSuccess: () => {
          toast.success(`«${concepto.nombre} › ${n}» lista para clasificar.`);
          setNueva("");
          setAgregando(false);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <Card>
      <CardHeader className="flex flex-wrap items-center justify-between gap-2">
        {editando ? (
          <form onSubmit={guardarNombre} className="flex flex-1 flex-wrap items-center gap-2">
            <div className="min-w-56 flex-1">
              <Input
                aria-label={`Nombre del rubro ${concepto.nombre}`}
                value={nombre}
                onChange={(e) => setNombre(e.target.value)}
              />
            </div>
            <Button type="submit" size="sm" loading={renombrarConcepto.isPending} disabled={!nombre.trim()}>
              Guardar
            </Button>
            <Button
              type="button"
              size="sm"
              variant="secondary"
              onClick={() => {
                setNombre(concepto.nombre);
                setEditando(false);
              }}
            >
              Cancelar
            </Button>
          </form>
        ) : (
          <>
            <CardTitle className="flex items-center gap-2">
              {concepto.nombre}
              <Badge tone="neutral">
                {clasificaciones.length === 1 ? "1 clasificación" : `${clasificaciones.length} clasificaciones`}
              </Badge>
            </CardTitle>
            <div className="flex gap-2">
              <Button size="sm" variant="secondary" onClick={() => setEditando(true)}>
                Renombrar
              </Button>
              <Button size="sm" variant={agregando ? "ghost" : "secondary"} onClick={() => setAgregando((v) => !v)}>
                {agregando ? "Cancelar" : "+ Clasificación"}
              </Button>
            </div>
          </>
        )}
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        {agregando && (
          <form onSubmit={agregarClasificacion} className="flex flex-wrap items-end gap-2 rounded-lg border border-dashed border-border p-2">
            <div className="min-w-56 flex-1">
              <Input
                label={`Nueva clasificación de «${concepto.nombre}»`}
                value={nueva}
                onChange={(e) => setNueva(e.target.value)}
                placeholder="Ej. Honorarios contables"
              />
            </div>
            <Button type="submit" size="sm" loading={crearClasif.isPending} disabled={!nueva.trim()}>
              Crear
            </Button>
          </form>
        )}

        {clasificaciones.length === 0 ? (
          <p className="text-sm text-content-muted">
            Este rubro todavía no tiene clasificaciones. El clasificador de facturas trabaja con
            «Rubro › Clasificación», así que hace falta al menos una.
          </p>
        ) : (
          <ul className="flex flex-col divide-y divide-border">
            {clasificaciones.map((cl) => (
              <FilaClasificacion key={cl.id} clasificacion={cl} />
            ))}
          </ul>
        )}
      </CardContent>
    </Card>
  );
}

function FilaClasificacion({ clasificacion }: { clasificacion: ClasificacionCatalogo }) {
  const toast = useToast();
  const renombrar = useRenombrarClasificacionGasto();
  const [editando, setEditando] = useState(false);
  const [nombre, setNombre] = useState(clasificacion.nombre);

  function guardar(e: FormEvent) {
    e.preventDefault();
    const n = nombre.trim();
    if (!n) return;
    renombrar.mutate(
      { id: clasificacion.id, nombre: n },
      {
        onSuccess: () => {
          toast.success("Clasificación renombrada.");
          setEditando(false);
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  return (
    <li className="flex flex-wrap items-center gap-2 py-1.5">
      {editando ? (
        <form onSubmit={guardar} className="flex flex-1 flex-wrap items-center gap-2">
          <div className="min-w-56 flex-1">
            <Input
              aria-label={`Nombre de la clasificación ${clasificacion.nombre}`}
              value={nombre}
              onChange={(e) => setNombre(e.target.value)}
            />
          </div>
          <Button type="submit" size="sm" loading={renombrar.isPending} disabled={!nombre.trim()}>
            Guardar
          </Button>
          <Button
            type="button"
            size="sm"
            variant="secondary"
            onClick={() => {
              setNombre(clasificacion.nombre);
              setEditando(false);
            }}
          >
            Cancelar
          </Button>
        </form>
      ) : (
        <>
          <span className="flex-1 text-sm text-content">{clasificacion.nombre}</span>
          <Button size="sm" variant="ghost" onClick={() => setEditando(true)}>
            Renombrar
          </Button>
        </>
      )}
    </li>
  );
}
