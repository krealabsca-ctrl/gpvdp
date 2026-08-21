/**
 * CxP — Departamentos (/cxp/departamentos).
 * Catálogo de áreas / centros de costo de la empresa. Se usan para segmentar el gasto
 * (además del Concepto/Clasificación) y son la base de la validación por departamento.
 * Alta/edición en un panel que se abre con botón; baja lógica (desactivar).
 */

import { useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
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
import { mensajeError } from "@/lib/apiError";
import {
  useActualizarDepartamento,
  useAsignarValidador,
  useCrearDepartamento,
  useDepartamentos,
  useDesactivarDepartamento,
  useQuitarValidador,
  useUsuariosEmpresa,
  useValidadores,
} from "@/features/cxp/hooks";
import type { Departamento } from "@/api/cxp";

interface FormState {
  nombre: string;
  codigo: string;
  centro_costo: string;
}
const VACIO: FormState = { nombre: "", codigo: "", centro_costo: "" };

export function DepartamentosPage() {
  const toast = useToast();
  const departamentosQuery = useDepartamentos(false);
  const crear = useCrearDepartamento();
  const actualizar = useActualizarDepartamento();
  const desactivar = useDesactivarDepartamento();

  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(VACIO);
  const [mostrarForm, setMostrarForm] = useState(false);
  const [validadoresDe, setValidadoresDe] = useState<Departamento | null>(null);

  const guardando = crear.isPending || actualizar.isPending;

  function set<K extends keyof FormState>(key: K, value: FormState[K]) {
    setForm((prev) => ({ ...prev, [key]: value }));
  }
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
  function editar(d: Departamento) {
    setEditId(d.id);
    setForm({ nombre: d.nombre, codigo: d.codigo, centro_costo: d.centro_costo });
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
    const input = {
      nombre,
      codigo: form.codigo.trim() || undefined,
      centro_costo: form.centro_costo.trim() || undefined,
    };
    const onOk = (verbo: string) => {
      toast.success(`Departamento ${verbo}.`);
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

  function desactivarDepartamento(d: Departamento) {
    desactivar.mutate(d.id, {
      onSuccess: () => toast.success(`Departamento "${d.nombre}" desactivado.`),
      onError: (err) => toast.error(mensajeError(err)),
    });
  }

  const departamentos = departamentosQuery.data ?? [];

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Departamentos"
        description="Áreas / centros de costo de la empresa. Segmentan el gasto y son la base de la validación por departamento."
        actions={
          <Button
            variant={mostrarForm && !editId ? "secondary" : "primary"}
            onClick={() => (mostrarForm ? limpiar() : abrirNuevo())}
          >
            {mostrarForm ? "Cerrar" : "+ Nuevo departamento"}
          </Button>
        }
      />

      {mostrarForm && (
        <Card>
          <CardHeader>
            <CardTitle>{editId ? "Editar departamento" : "Nuevo departamento"}</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSubmit} className="grid grid-cols-1 gap-3 sm:grid-cols-3">
              <Input
                label="Nombre *"
                value={form.nombre}
                onChange={(e) => set("nombre", e.target.value)}
                placeholder="Ej. Logística, Mercadeo…"
              />
              <Input
                label="Código"
                value={form.codigo}
                onChange={(e) => set("codigo", e.target.value)}
                placeholder="LOG"
              />
              <Input
                label="Centro de costo"
                value={form.centro_costo}
                onChange={(e) => set("centro_costo", e.target.value)}
                placeholder="5-01"
              />
              <div className="flex items-end gap-2 sm:col-span-3">
                <Button type="submit" loading={guardando} disabled={!form.nombre.trim()}>
                  {editId ? "Guardar cambios" : "Crear departamento"}
                </Button>
                <Button type="button" variant="secondary" onClick={limpiar} disabled={guardando}>
                  Cancelar
                </Button>
              </div>
            </form>
          </CardContent>
        </Card>
      )}

      {departamentosQuery.isPending ? (
        <LoadingState label="Cargando departamentos" />
      ) : departamentosQuery.isError ? (
        <ErrorState
          message={mensajeError(departamentosQuery.error)}
          onRetry={() => departamentosQuery.refetch()}
        />
      ) : departamentos.length === 0 ? (
        <EmptyState message="No hay departamentos. Creá el primero." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Departamento</TH>
                <TH>Código</TH>
                <TH>Centro de costo</TH>
                <TH>Estado</TH>
                <TH className="text-right">Acciones</TH>
              </TR>
            </THead>
            <TBody>
              {departamentos.map((d) => (
                <TR key={d.id} className={d.activo ? undefined : "opacity-60"}>
                  <TD className="font-medium">{d.nombre}</TD>
                  <TD className="tabular-nums">{d.codigo || "—"}</TD>
                  <TD className="tabular-nums">{d.centro_costo || "—"}</TD>
                  <TD>
                    {d.activo ? (
                      <Badge tone="positivo">Activo</Badge>
                    ) : (
                      <Badge tone="negativo">Inactivo</Badge>
                    )}
                  </TD>
                  <TD className="text-right">
                    <div className="flex justify-end gap-2">
                      <Button size="sm" variant="secondary" onClick={() => setValidadoresDe(d)}>
                        Validadores
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => editar(d)}>
                        Editar
                      </Button>
                      {d.activo && (
                        <Button
                          size="sm"
                          variant="ghost"
                          onClick={() => desactivarDepartamento(d)}
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

      {validadoresDe && (
        <ValidadoresDialog departamento={validadoresDe} onCerrar={() => setValidadoresDe(null)} />
      )}
    </div>
  );
}

// ---------------------------------------------------------------------------
// Modal: validadores de un departamento (titular / suplente)
// ---------------------------------------------------------------------------
function ValidadoresDialog({ departamento, onCerrar }: { departamento: Departamento; onCerrar: () => void }) {
  const toast = useToast();
  const validadoresQ = useValidadores(departamento.id);
  const usuariosQ = useUsuariosEmpresa();
  const asignar = useAsignarValidador();
  const quitar = useQuitarValidador();
  const [usuarioId, setUsuarioId] = useState("");
  const [rol, setRol] = useState<"TITULAR" | "SUPLENTE">("TITULAR");

  const validadores = validadoresQ.data ?? [];
  const yaAsignados = new Set(validadores.map((v) => v.usuario_id));
  const disponibles = (usuariosQ.data ?? []).filter((u) => !yaAsignados.has(u.id));
  const sinPermisoUsuarios = usuariosQ.isError; // 403: no puede administrar validadores

  function agregar() {
    if (!usuarioId) return;
    asignar.mutate(
      { departamentoId: departamento.id, usuarioId, rol },
      {
        onSuccess: () => {
          setUsuarioId("");
          toast.success("Validador asignado.");
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }
  function remover(uid: string) {
    quitar.mutate(
      { departamentoId: departamento.id, usuarioId: uid },
      { onSuccess: () => toast.success("Validador quitado."), onError: (e) => toast.error(mensajeError(e)) },
    );
  }

  return createPortal(
    <div
      className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4"
      onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}
    >
      <div className="w-full max-w-lg rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="mb-1 text-base font-semibold text-content">Validadores — {departamento.nombre}</h2>
        <p className="mb-4 text-xs text-content-muted">
          Titular y suplente(s) que pueden validar las facturas de esta área. El suplente cubre ausencias.
        </p>

        {validadoresQ.isPending ? (
          <LoadingState label="Cargando validadores" />
        ) : validadores.length === 0 ? (
          <div className="mb-4 rounded-lg border border-pendiente/40 bg-pendiente/10 px-3 py-2 text-sm text-content">
            ⚠ Sin validadores. Un departamento sin validador dejaría sus facturas sin poder avanzar.
          </div>
        ) : (
          <div className="mb-4 flex flex-col gap-1.5">
            {validadores.map((v) => (
              <div key={v.usuario_id} className="flex items-center gap-2 rounded-lg border border-border px-3 py-2">
                <Badge tone={v.rol === "TITULAR" ? "accent" : "neutral"}>{v.rol === "TITULAR" ? "Titular" : "Suplente"}</Badge>
                <span className="text-sm font-medium text-content">{v.nombre}</span>
                <span className="text-xs text-content-muted">{v.email}</span>
                <Button
                  size="sm"
                  variant="ghost"
                  className="ml-auto text-negativo"
                  loading={quitar.isPending}
                  onClick={() => remover(v.usuario_id)}
                >
                  Quitar
                </Button>
              </div>
            ))}
          </div>
        )}

        {sinPermisoUsuarios ? (
          <div className="border-t border-border pt-4 text-sm text-content-muted">
            Solo un administrador (Dirección/Admin) puede asignar validadores.
          </div>
        ) : (
          <div className="flex flex-wrap items-end gap-2 border-t border-border pt-4">
            <div className="min-w-48 flex-1">
              <label className="mb-1 block text-xs font-medium text-content">Agregar validador</label>
              <Select
                aria-label="Usuario"
                value={usuarioId}
                onChange={(e) => setUsuarioId(e.target.value)}
                options={[
                  { value: "", label: disponibles.length ? "Seleccioná un usuario…" : "Sin usuarios disponibles" },
                  ...disponibles.map((u) => ({ value: u.id, label: `${u.nombre} (${u.email})` })),
                ]}
              />
            </div>
            <Select
              aria-label="Rol"
              value={rol}
              onChange={(e) => setRol(e.target.value as "TITULAR" | "SUPLENTE")}
              options={[
                { value: "TITULAR", label: "Titular" },
                { value: "SUPLENTE", label: "Suplente" },
              ]}
              className="w-36"
            />
            <Button onClick={agregar} disabled={!usuarioId} loading={asignar.isPending}>
              Agregar
            </Button>
          </div>
        )}

        <div className="mt-5 flex justify-end">
          <Button variant="secondary" onClick={onCerrar}>
            Cerrar
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
