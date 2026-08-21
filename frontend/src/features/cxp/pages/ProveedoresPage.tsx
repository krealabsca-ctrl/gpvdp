/**
 * CxP — Proveedores (/cxp/proveedores).
 * Maestro: alta/edición en un panel que se abre con botón (el área queda limpia),
 * barra de filtros por todos los criterios de la tabla y baja lógica (desactivar).
 * El IBAN y la retención de renta se usan luego en el archivo de pago (SINPE).
 * El "Departamento" es un segmento adicional al gasto: ordena el gasto por área
 * de la empresa (Logística, Mercadeo, Ventas, Cobros, Finanzas…) para reportes.
 */

import { useState, type FormEvent } from "react";
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
  TBody,
  TD,
  TH,
  THead,
  Table,
  TableContainer,
  TR,
  useToast,
} from "@/components/ui";
import { toNumber } from "@/lib/format";
import { mensajeError } from "@/lib/apiError";
import {
  useActualizarProveedor,
  useCrearProveedor,
  useDepartamentos,
  useDesactivarProveedor,
  useMarcarProveedorContabilidad,
  useProveedores,
  useSubclasificacionesTodas,
} from "@/features/cxp/hooks";
import { useClasificaciones, useConceptos } from "@/features/bancos/hooks";
import { useTienePermiso } from "@/features/auth/permisos";
import { CargarIBANPanel } from "@/features/cxp/components/CargarIBANPanel";
import { GastoCombobox, type GastoElegido } from "@/features/cxp/components/GastoCombobox";
import type { FiltrosProveedores, Proveedor, ProveedorInput } from "@/api/cxp";

const TIPO_OPTIONS = [
  { value: "FISICA", label: "Física" },
  { value: "JURIDICA", label: "Jurídica" },
  { value: "DIMEX", label: "DIMEX" },
  { value: "NITE", label: "NITE" },
];

// Opciones de la barra de filtros (la primera, value "", es "todos" y limpia el criterio).
// Los departamentos vienen del catálogo (useDepartamentos), administrable en /cxp/departamentos.
const OPT_ESTADO = [
  { value: "", label: "Estado: todos" },
  { value: "activo", label: "Activos" },
  { value: "inactivo", label: "Inactivos" },
];
const OPT_IVA = [
  { value: "", label: "IVA: todos" },
  { value: "grava", label: "Grava IVA" },
  { value: "exento", label: "Exento" },
];
const OPT_COND = [
  { value: "", label: "Condición: todas" },
  { value: "CONTADO", label: "Contado" },
  { value: "CREDITO", label: "Crédito" },
];
const OPT_RET = [
  { value: "", label: "Retención: todas" },
  { value: "con", label: "Con retención" },
  { value: "sin", label: "Sin retención" },
];
const OPT_IBAN = [
  { value: "", label: "IBAN: todos" },
  { value: "con", label: "Con IBAN" },
  { value: "sin", label: "Sin IBAN" },
];
const OPT_GASTO = [
  { value: "", label: "Gasto AUTO: todos" },
  { value: "con", label: "Con gasto" },
  { value: "sin", label: "Sin gasto" },
];

interface FormState {
  nombre: string;
  tipo_identificacion: string;
  identificacion: string;
  email: string;
  telefono: string;
  iban: string;
  retencion_renta_pct: string;
  exento_iva: boolean;
  condicion_pago: string;
  plazo_credito_dias: string;
  departamento: string;
  gasto: GastoElegido | null;
}

const VACIO: FormState = {
  nombre: "",
  tipo_identificacion: "",
  identificacion: "",
  email: "",
  telefono: "",
  iban: "",
  retencion_renta_pct: "",
  exento_iva: false,
  condicion_pago: "CONTADO",
  plazo_credito_dias: "0",
  departamento: "",
  gasto: null,
};

const PAGE_SIZE = 50;

export function ProveedoresPage() {
  const toast = useToast();
  const tiene = useTienePermiso();
  const [filtros, setFiltros] = useState<FiltrosProveedores>({});
  const [page, setPage] = useState(1);
  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(VACIO);
  const [mostrarForm, setMostrarForm] = useState(false);

  const proveedoresQuery = useProveedores(filtros, page, PAGE_SIZE);
  const departamentosQuery = useDepartamentos(true); // solo activos, para los selects
  const deptoOpts = (departamentosQuery.data ?? []).map((d) => ({ value: d.nombre, label: d.nombre }));
  const crear = useCrearProveedor();
  const actualizar = useActualizarProveedor();
  const desactivar = useDesactivarProveedor();
  const marcarContaM = useMarcarProveedorContabilidad();
  const puedeMarcarConta = tiene("cxp.marcar_contabilidad");
  // Cargar cuentas IBAN es editar el maestro de proveedores: mismo permiso.
  const puedeEditarProveedores = tiene("cxp.proveedores");

  // Catálogo de gasto (para mostrar la ruta del gasto predeterminado).
  const conceptosQ = useConceptos("cxp");
  const clasifsQ = useClasificaciones("cxp");
  const subsQ = useSubclasificacionesTodas();
  function rutaGasto(p: Proveedor): string {
    if (!p.gasto_concepto_id) return "";
    const con = (conceptosQ.data ?? []).find((c) => c.id === p.gasto_concepto_id)?.nombre;
    const cla = (clasifsQ.data ?? []).find((c) => c.id === p.gasto_clasificacion_id)?.nombre;
    const sub = (subsQ.data ?? []).find((s) => s.id === p.gasto_subclasificacion_id)?.nombre;
    return [con, cla, sub].filter(Boolean).join(" › ");
  }

  const guardando = crear.isPending || actualizar.isPending;

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }

  // Filtros: value vacío limpia el criterio. Cualquier cambio vuelve a la página 1.
  function setFiltro(key: keyof FiltrosProveedores, value: string) {
    setFiltros((prev) => {
      const next = { ...prev } as Record<string, string>;
      if (value) next[key] = value;
      else delete next[key];
      return next as FiltrosProveedores;
    });
    setPage(1);
  }
  const filtrosActivos = Object.keys(filtros).length;

  function limpiar() {
    setEditId(null);
    setForm(VACIO);
    setMostrarForm(false);
  }

  function abrirNuevo() {
    setEditId(null);
    setForm(VACIO);
    setMostrarForm(true);
  }

  function editar(p: Proveedor) {
    setEditId(p.id);
    setForm({
      nombre: p.nombre,
      tipo_identificacion: p.tipo_identificacion,
      identificacion: p.identificacion,
      email: p.email,
      telefono: p.telefono,
      iban: p.iban,
      retencion_renta_pct: p.retencion_renta_pct,
      exento_iva: p.exento_iva,
      condicion_pago: p.condicion_pago || "CONTADO",
      plazo_credito_dias: String(p.plazo_credito_dias ?? 0),
      departamento: p.departamento || "",
      gasto: p.gasto_concepto_id
        ? {
            conceptoId: p.gasto_concepto_id,
            clasificacionId: p.gasto_clasificacion_id,
            subclasificacionId: p.gasto_subclasificacion_id,
            ruta: rutaGasto(p),
          }
        : null,
    });
    setMostrarForm(true);
    if (typeof window !== "undefined") window.scrollTo({ top: 0, behavior: "smooth" });
  }

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    const nombre = form.nombre.trim();
    if (!nombre) {
      toast.error("El nombre es obligatorio.");
      return;
    }
    const input: ProveedorInput = {
      nombre,
      tipo_identificacion: form.tipo_identificacion || undefined,
      identificacion: form.identificacion.trim() || undefined,
      email: form.email.trim() || undefined,
      telefono: form.telefono.trim() || undefined,
      iban: form.iban.trim() || undefined,
      retencion_renta_pct: form.retencion_renta_pct.trim() || undefined,
      exento_iva: form.exento_iva,
      condicion_pago: form.condicion_pago,
      plazo_credito_dias: form.condicion_pago === "CREDITO" ? Number(form.plazo_credito_dias) || 0 : 0,
      departamento: form.departamento || undefined,
      gasto_concepto_id: form.gasto?.conceptoId || undefined,
      gasto_clasificacion_id: form.gasto?.clasificacionId || undefined,
      gasto_subclasificacion_id: form.gasto?.subclasificacionId || undefined,
    };
    const onOk = (verbo: string) => {
      toast.success(`Proveedor ${verbo}.`);
      limpiar();
    };
    if (editId) {
      actualizar.mutate(
        { id: editId, input },
        { onSuccess: () => onOk("actualizado"), onError: (err) => toast.error(mensajeError(err)) },
      );
    } else {
      crear.mutate(input, {
        onSuccess: () => onOk("creado"),
        onError: (err) => toast.error(mensajeError(err)),
      });
    }
  }

  function desactivarProveedor(p: Proveedor) {
    desactivar.mutate(p.id, {
      onSuccess: () => toast.success(`Proveedor "${p.nombre}" desactivado.`),
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  /**
   * Marca al proveedor como «de Contabilidad»: sus facturas dejan de requerir validación de área.
   * Es RETROACTIVO sobre lo que todavía no se aprobó, y eso es lo que se quiere —esas son
   * justamente las facturas que están trancadas—, así que se avisa en el mensaje.
   */
  function marcarConta(p: Proveedor) {
    const valor = !p.es_contabilidad;
    marcarContaM.mutate(
      { id: p.id, valor },
      {
        onSuccess: () =>
          toast.success(
            valor
              ? `"${p.nombre}" es de Contabilidad: sus facturas abiertas ya no esperan validación de área.`
              : `"${p.nombre}" vuelve al flujo normal: sus facturas requieren validación de área.`,
          ),
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const proveedores = proveedoresQuery.data?.items ?? [];
  const total = proveedoresQuery.data?.total ?? 0;
  const totalPages = Math.max(1, Math.ceil(total / PAGE_SIZE));

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Proveedores"
        description="Maestro de proveedores de la empresa activa. El IBAN y la retención se usan en el archivo de pago."
        actions={
          <Button variant={mostrarForm && !editId ? "secondary" : "primary"} onClick={() => (mostrarForm ? limpiar() : abrirNuevo())}>
            {mostrarForm ? "Cerrar" : "+ Nuevo proveedor"}
          </Button>
        }
      />

      {/* La cuenta IBAN es lo que hace pagable a un proveedor: si falta, el banco rechaza la
          línea de la macro. Va arriba del maestro porque hoy es el faltante que trancaba los pagos. */}
      {puedeEditarProveedores && <CargarIBANPanel />}

      {/* Alta / edición — se abre con el botón para mantener el área limpia. */}
      {mostrarForm && (
        <Card>
          <CardHeader>
            <CardTitle>{editId ? "Editar proveedor" : "Nuevo proveedor"}</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSubmit} className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <Input
                label="Nombre *"
                value={form.nombre}
                onChange={(e) => set("nombre", e.target.value)}
                placeholder="Razón social o nombre"
              />
              <Select
                label="Tipo de identificación"
                placeholder="Seleccioná…"
                value={form.tipo_identificacion}
                onChange={(e) => set("tipo_identificacion", e.target.value)}
                options={TIPO_OPTIONS}
              />
              <Input
                label="Identificación"
                value={form.identificacion}
                onChange={(e) => set("identificacion", e.target.value)}
                placeholder="Cédula / DIMEX / NITE"
              />
              <Input
                label="Email"
                type="email"
                value={form.email}
                onChange={(e) => set("email", e.target.value)}
                placeholder="proveedor@correo.com"
              />
              <Input
                label="Teléfono"
                value={form.telefono}
                onChange={(e) => set("telefono", e.target.value)}
                placeholder="0000-0000"
              />
              <Input
                label="IBAN"
                value={form.iban}
                onChange={(e) => set("iban", e.target.value)}
                placeholder="CR00 0000 0000 0000 0000 00"
                className="font-mono"
              />
              <Input
                label="Retención renta (%)"
                value={form.retencion_renta_pct}
                onChange={(e) => set("retencion_renta_pct", e.target.value)}
                placeholder="0"
                inputMode="decimal"
              />
              <Select
                label="Condición de pago"
                value={form.condicion_pago}
                onChange={(e) => set("condicion_pago", e.target.value)}
                options={[
                  { value: "CONTADO", label: "Contado" },
                  { value: "CREDITO", label: "Crédito" },
                ]}
              />
              {form.condicion_pago === "CREDITO" && (
                <Input
                  label="Plazo de crédito (días)"
                  value={form.plazo_credito_dias}
                  onChange={(e) => set("plazo_credito_dias", e.target.value)}
                  placeholder="30"
                  inputMode="numeric"
                />
              )}
              <Select
                label="Departamento"
                value={form.departamento}
                onChange={(e) => set("departamento", e.target.value)}
                options={[{ value: "", label: "— Sin departamento —" }, ...deptoOpts]}
              />
              <div className="flex flex-col gap-1.5">
                <span className="text-sm font-medium text-content">Gasto predeterminado (AUTO)</span>
                <div className="flex items-center gap-2">
                  <GastoCombobox
                    actual={form.gasto?.ruta ?? ""}
                    proveedorId={editId ?? undefined}
                    onElegir={(g) => set("gasto", g)}
                  />
                  {form.gasto && (
                    <Button type="button" variant="ghost" size="sm" onClick={() => set("gasto", null)}>
                      Quitar
                    </Button>
                  )}
                </div>
                <span className="text-[11px] text-content-muted">
                  Sus facturas nacerán pre-clasificadas con esto. Podés buscar o crear una categoría nueva ahí mismo.
                </span>
              </div>
              <label className="flex items-center gap-2 self-end pb-2.5 text-sm text-content">
                <input
                  type="checkbox"
                  checked={form.exento_iva}
                  onChange={(e) => set("exento_iva", e.target.checked)}
                  className="h-4 w-4 rounded border-border accent-accent"
                />
                Exento de IVA
              </label>
              <div className="flex items-end gap-2">
                <Button type="submit" loading={guardando} disabled={!form.nombre.trim()}>
                  {editId ? "Guardar cambios" : "Crear proveedor"}
                </Button>
                <Button type="button" variant="secondary" onClick={limpiar} disabled={guardando}>
                  Cancelar
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {/* Filtros — por todos los criterios de la tabla. */}
      <Card>
        <CardContent className="flex flex-col gap-3 pt-5">
          <div className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-4">
            <Input
              label="Buscar"
              value={filtros.q ?? ""}
              onChange={(e) => setFiltro("q", e.target.value)}
              placeholder="Nombre o identificación…"
            />
            <Select
              label="Departamento"
              value={filtros.departamento ?? ""}
              onChange={(e) => setFiltro("departamento", e.target.value)}
              options={[{ value: "", label: "Departamento: todos" }, ...deptoOpts]}
            />
            <Select
              label="Gasto (AUTO)"
              value={filtros.gasto ?? ""}
              onChange={(e) => setFiltro("gasto", e.target.value)}
              options={OPT_GASTO}
            />
            <Select
              label="Condición"
              value={filtros.condicion ?? ""}
              onChange={(e) => setFiltro("condicion", e.target.value)}
              options={OPT_COND}
            />
            <Select
              label="IBAN"
              value={filtros.iban ?? ""}
              onChange={(e) => setFiltro("iban", e.target.value)}
              options={OPT_IBAN}
            />
            <Select
              label="Retención"
              value={filtros.retencion ?? ""}
              onChange={(e) => setFiltro("retencion", e.target.value)}
              options={OPT_RET}
            />
            <Select
              label="IVA"
              value={filtros.iva ?? ""}
              onChange={(e) => setFiltro("iva", e.target.value)}
              options={OPT_IVA}
            />
            <Select
              label="Estado"
              value={filtros.estado ?? ""}
              onChange={(e) => setFiltro("estado", e.target.value)}
              options={OPT_ESTADO}
            />
          </div>
          {filtrosActivos > 0 && (
            <div className="flex items-center gap-3">
              <span className="text-xs text-content-muted">
                {filtrosActivos} filtro(s) activo(s)
              </span>
              <Button
                variant="ghost"
                size="sm"
                onClick={() => {
                  setFiltros({});
                  setPage(1);
                }}
              >
                Limpiar filtros
              </Button>
            </div>
          )}
        </CardContent>
      </Card>

      {proveedoresQuery.isPending ? (
        <LoadingState label="Cargando proveedores" />
      ) : proveedoresQuery.isError ? (
        <ErrorState
          message={mensajeError(proveedoresQuery.error)}
          onRetry={() => proveedoresQuery.refetch()}
        />
      ) : proveedores.length === 0 ? (
        <EmptyState message="No hay proveedores para estos filtros." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Nombre</TH>
                <TH>Identificación</TH>
                <TH>Departamento</TH>
                <TH>Gasto (AUTO)</TH>
                <TH>IBAN</TH>
                <TH>Condición</TH>
                <TH className="text-right">Retención</TH>
                <TH>IVA</TH>
                <TH>Estado</TH>
                <TH className="text-right">Acciones</TH>
              </TR>
            </THead>
            <TBody>
              {proveedores.map((p) => (
                <TR key={p.id} className={p.activo ? undefined : "opacity-60"}>
                  <TD className="font-medium">{p.nombre}</TD>
                  <TD>
                    {p.identificacion ? (
                      <span>
                        {p.identificacion}
                        {p.tipo_identificacion && (
                          <span className="ml-1 text-xs text-content-muted">
                            ({p.tipo_identificacion})
                          </span>
                        )}
                      </span>
                    ) : (
                      "—"
                    )}
                  </TD>
                  <TD>
                    {p.departamento ? (
                      <Badge tone="neutral">{p.departamento}</Badge>
                    ) : (
                      <span className="text-content-muted">—</span>
                    )}
                    {/* La marca «de Contabilidad» va junto al área porque es la respuesta a la
                        misma pregunta: quién confirma este gasto. Acá nadie: es de Contabilidad. */}
                    {p.es_contabilidad && (
                      <span className="mt-1 block" title="Sus facturas no requieren validación de área">
                        <Badge tone="accent">🧾 De Contabilidad</Badge>
                      </span>
                    )}
                  </TD>
                  <TD className="max-w-52 text-sm">
                    {rutaGasto(p) ? (
                      <span className="truncate" title={rutaGasto(p)}>{rutaGasto(p)}</span>
                    ) : (
                      <span className="text-content-muted">—</span>
                    )}
                  </TD>
                  <TD className="font-mono text-xs">{p.iban || "—"}</TD>
                  <TD>
                    {p.condicion_pago === "CREDITO" ? (
                      <Badge tone="pendiente">Crédito {p.plazo_credito_dias}d</Badge>
                    ) : (
                      <Badge tone="neutral">Contado</Badge>
                    )}
                  </TD>
                  <TD className="text-right tabular-nums">{toNumber(p.retencion_renta_pct)}%</TD>
                  <TD>
                    {p.exento_iva ? <Badge tone="neutral">Exento</Badge> : <Badge tone="accent">Grava</Badge>}
                  </TD>
                  <TD>
                    {p.activo ? (
                      <Badge tone="positivo">Activo</Badge>
                    ) : (
                      <Badge tone="negativo">Inactivo</Badge>
                    )}
                  </TD>
                  <TD className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button size="sm" variant="secondary" onClick={() => editar(p)}>
                        Editar
                      </Button>
                      {puedeMarcarConta && (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => marcarConta(p)}
                          loading={marcarContaM.isPending}
                          title={
                            p.es_contabilidad
                              ? "Sus facturas volverán a requerir validación de área"
                              : "Sus facturas dejarán de requerir validación de área"
                          }
                        >
                          {p.es_contabilidad ? "Quitar de Conta" : "Es de Conta"}
                        </Button>
                      )}
                      {p.activo && (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => desactivarProveedor(p)}
                          loading={desactivar.isPending}
                        >
                          Desactivar
                        </Button>
                      )}
                    </div>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {!proveedoresQuery.isPending && !proveedoresQuery.isError && proveedores.length > 0 && (
        <div className="flex flex-wrap items-center justify-between gap-3">
          <p className="text-sm text-content-muted">
            {total} proveedor(es){" "}
            {proveedoresQuery.isFetching && <span className="text-accent">· actualizando…</span>}
          </p>
          <div className="flex items-center gap-2">
            <Button
              variant="secondary"
              size="sm"
              disabled={page <= 1}
              onClick={() => setPage((p) => Math.max(1, p - 1))}
            >
              Anterior
            </Button>
            <span className="text-sm tabular-nums text-content-muted">
              Página {page} de {totalPages}
            </span>
            <Button
              variant="secondary"
              size="sm"
              disabled={page >= totalPages}
              onClick={() => setPage((p) => p + 1)}
            >
              Siguiente
            </Button>
          </div>
        </div>
      )}
    </div>
  );
}
