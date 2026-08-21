/**
 * Administración — Usuarios (/usuarios): gestión de personas y su acceso a la empresa activa.
 * Requiere el permiso admin.roles. Contraseña TEMPORAL fijada por el admin (el usuario la
 * cambia al primer ingreso). La gestión es por empresa activa (multi-tenant).
 */

import { useState, type FormEvent } from "react";
import { createPortal } from "react-dom";
import { useMutation, useQuery, useQueryClient } from "@tanstack/react-query";
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
import { useAuth } from "@/features/auth/AuthContext";
import { rbacApi, type UsuarioAdmin } from "@/api/rbac";

interface FormState {
  nombre: string;
  email: string;
  password: string;
  rol_codigo: string;
  activo: boolean;
}
const VACIO: FormState = { nombre: "", email: "", password: "", rol_codigo: "", activo: true };

export function UsuariosPage() {
  const toast = useToast();
  const qc = useQueryClient();
  const { empresaActiva } = useAuth();
  const empId = empresaActiva?.id ?? "none";

  const usuariosQ = useQuery({ queryKey: ["rbac", "usuarios", empId], queryFn: () => rbacApi.usuarios() });
  const rolesQ = useQuery({ queryKey: ["rbac", "roles", empId], queryFn: () => rbacApi.roles() });

  const [editId, setEditId] = useState<string | null>(null);
  const [form, setForm] = useState<FormState>(VACIO);
  const [mostrarForm, setMostrarForm] = useState(false);
  const [reset, setReset] = useState<UsuarioAdmin | null>(null);

  const rolOptions = (rolesQ.data ?? []).map((r) => ({ value: r.codigo, label: r.nombre }));
  const usuarios = usuariosQ.data ?? [];

  function set<K extends keyof FormState>(k: K, v: FormState[K]) {
    setForm((p) => ({ ...p, [k]: v }));
  }
  function refrescar() {
    void qc.invalidateQueries({ queryKey: ["rbac", "usuarios", empId] });
  }
  function limpiar() {
    setEditId(null);
    setForm(VACIO);
    setMostrarForm(false);
  }
  function abrirNuevo() {
    setEditId(null);
    setForm({ ...VACIO, rol_codigo: rolOptions[0]?.value ?? "" });
    setMostrarForm(true);
  }
  function editar(u: UsuarioAdmin) {
    setEditId(u.id);
    setForm({ nombre: u.nombre, email: u.email, password: "", rol_codigo: u.rol_codigo, activo: u.activo });
    setMostrarForm(true);
  }

  const crear = useMutation({
    mutationFn: () => rbacApi.crearUsuario({ nombre: form.nombre.trim(), email: form.email.trim(), password: form.password, rol_codigo: form.rol_codigo }),
    onSuccess: (r) => {
      refrescar();
      limpiar();
      toast.success(r.nuevo ? "Usuario creado con contraseña temporal." : "Usuario existente: se le dio acceso a esta empresa.");
    },
    onError: (e) => toast.error(mensajeError(e)),
  });
  const actualizar = useMutation({
    mutationFn: () => rbacApi.actualizarUsuario(editId!, { nombre: form.nombre.trim(), activo: form.activo, rol_codigo: form.rol_codigo }),
    onSuccess: () => {
      refrescar();
      limpiar();
      toast.success("Usuario actualizado.");
    },
    onError: (e) => toast.error(mensajeError(e)),
  });
  const toggleActivo = useMutation({
    mutationFn: (u: UsuarioAdmin) => rbacApi.actualizarUsuario(u.id, { nombre: u.nombre, activo: !u.activo, rol_codigo: u.rol_codigo }),
    onSuccess: (_d, u) => {
      refrescar();
      toast.success(u.activo ? "Usuario desactivado." : "Usuario reactivado.");
    },
    onError: (e) => toast.error(mensajeError(e)),
  });
  const quitarAcceso = useMutation({
    mutationFn: (u: UsuarioAdmin) => rbacApi.quitarAcceso(u.id),
    onSuccess: () => {
      refrescar();
      toast.success("Acceso a esta empresa retirado.");
    },
    onError: (e) => toast.error(mensajeError(e)),
  });
  const aplicarFaltantes = useMutation({
    mutationFn: () => rbacApi.aplicarPermisosFaltantes(),
    onSuccess: (r) => {
      void qc.invalidateQueries({ queryKey: ["rbac"] });
      toast.success(r.agregados > 0 ? `${r.agregados} permiso(s) nuevo(s) aplicado(s) a los roles base.` : "Los roles ya tenían todos los permisos por defecto.");
    },
    onError: (e) => toast.error(mensajeError(e)),
  });

  function onSubmit(e: FormEvent) {
    e.preventDefault();
    if (!form.nombre.trim()) return toast.error("El nombre es obligatorio.");
    if (!form.rol_codigo) return toast.error("Elegí un rol.");
    if (editId) {
      actualizar.mutate();
    } else {
      if (!form.email.trim()) return toast.error("El correo es obligatorio.");
      if (form.password.length < 8) return toast.error("La contraseña temporal debe tener al menos 8 caracteres.");
      crear.mutate();
    }
  }

  const guardando = crear.isPending || actualizar.isPending;
  const cargando = usuariosQ.isPending || rolesQ.isPending;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Administración · Usuarios"
        description={`Personas con acceso a ${empresaActiva?.nombre ?? "la empresa"} y su rol. La contraseña inicial es temporal; el usuario la cambia al ingresar.`}
        actions={
          <div className="flex flex-wrap gap-2">
            <Button variant="secondary" onClick={() => aplicarFaltantes.mutate()} loading={aplicarFaltantes.isPending}>
              Aplicar permisos nuevos
            </Button>
            <Button variant={mostrarForm && !editId ? "secondary" : "primary"} onClick={() => (mostrarForm ? limpiar() : abrirNuevo())}>
              {mostrarForm ? "Cerrar" : "+ Nuevo usuario"}
            </Button>
          </div>
        }
      />

      {mostrarForm && (
        <Card>
          <CardHeader>
            <CardTitle>{editId ? "Editar usuario" : "Nuevo usuario"}</CardTitle>
          </CardHeader>
          <CardContent>
            <form onSubmit={onSubmit} className="grid grid-cols-1 gap-3 sm:grid-cols-2 lg:grid-cols-3">
              <Input label="Nombre *" value={form.nombre} onChange={(e) => set("nombre", e.target.value)} placeholder="Nombre y apellidos" />
              <Input
                label="Correo *"
                type="email"
                value={form.email}
                onChange={(e) => set("email", e.target.value)}
                placeholder="persona@valledepazcr.com"
                disabled={!!editId}
              />
              <Select label="Rol en esta empresa *" value={form.rol_codigo} onChange={(e) => set("rol_codigo", e.target.value)} placeholder="Elegí un rol" options={rolOptions} />
              {!editId && (
                <Input
                  label="Contraseña temporal *"
                  value={form.password}
                  onChange={(e) => set("password", e.target.value)}
                  placeholder="Mínimo 8 caracteres"
                />
              )}
              {editId && (
                <label className="flex items-center gap-2 self-end pb-2.5 text-sm text-content">
                  <input type="checkbox" checked={form.activo} onChange={(e) => set("activo", e.target.checked)} className="h-4 w-4 rounded border-border accent-accent" />
                  Usuario activo
                </label>
              )}
              <div className="flex items-end gap-2">
                <Button type="submit" loading={guardando} disabled={!form.nombre.trim()}>
                  {editId ? "Guardar cambios" : "Crear usuario"}
                </Button>
                <Button type="button" variant="secondary" onClick={limpiar} disabled={guardando}>
                  Cancelar
                </Button>
              </div>
            </form>
            {!editId && (
              <p className="mt-2 text-[11px] text-content-muted">
                El usuario ingresa con esta contraseña temporal y el sistema le exige cambiarla en el primer acceso.
              </p>
            )}
          </CardContent>
        </Card>
      )}

      {cargando ? (
        <LoadingState label="Cargando usuarios" />
      ) : usuariosQ.isError ? (
        <ErrorState message={mensajeError(usuariosQ.error)} onRetry={() => usuariosQ.refetch()} />
      ) : usuarios.length === 0 ? (
        <EmptyState message="No hay usuarios con acceso a esta empresa. Creá el primero." />
      ) : (
        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Usuario</TH>
                <TH>Rol</TH>
                <TH>Estado</TH>
                <TH className="text-right">Acciones</TH>
              </TR>
            </THead>
            <TBody>
              {usuarios.map((u) => (
                <TR key={u.id} className={u.activo ? undefined : "opacity-60"}>
                  <TD>
                    <div className="font-medium">{u.nombre}</div>
                    <div className="text-xs text-content-muted">{u.email}</div>
                  </TD>
                  <TD>
                    <Badge tone="accent">{u.rol_nombre}</Badge>
                    {u.debe_cambiar_password && (
                      <span className="ml-2 text-[11px] text-pendiente">· contraseña temporal pendiente</span>
                    )}
                  </TD>
                  <TD>
                    {u.activo ? <Badge tone="positivo">Activo</Badge> : <Badge tone="negativo">Inactivo</Badge>}
                  </TD>
                  <TD className="text-right">
                    <div className="flex flex-wrap justify-end gap-2">
                      <Button size="sm" variant="secondary" onClick={() => editar(u)}>
                        Editar
                      </Button>
                      <Button size="sm" variant="secondary" onClick={() => setReset(u)}>
                        Restablecer contraseña
                      </Button>
                      <Button size="sm" variant="ghost" onClick={() => toggleActivo.mutate(u)} loading={toggleActivo.isPending}>
                        {u.activo ? "Desactivar" : "Reactivar"}
                      </Button>
                      <Button size="sm" variant="ghost" className="text-negativo" onClick={() => quitarAcceso.mutate(u)} loading={quitarAcceso.isPending}>
                        Quitar acceso
                      </Button>
                    </div>
                  </TD>
                </TR>
              ))}
            </TBody>
          </Table>
        </TableContainer>
      )}

      {reset && <ResetDialog usuario={reset} onCerrar={() => setReset(null)} onListo={refrescar} />}
    </div>
  );
}

function ResetDialog({ usuario, onCerrar, onListo }: { usuario: UsuarioAdmin; onCerrar: () => void; onListo: () => void }) {
  const toast = useToast();
  const [password, setPassword] = useState("");
  const m = useMutation({
    mutationFn: () => rbacApi.resetPassword(usuario.id, password),
    onSuccess: () => {
      onListo();
      onCerrar();
      toast.success("Contraseña temporal fijada — el usuario la cambiará al ingresar.");
    },
    onError: (e) => toast.error(mensajeError(e)),
  });
  return createPortal(
    <div className="fixed inset-0 z-[95] flex items-center justify-center bg-black/40 p-4" onMouseDown={(e) => e.target === e.currentTarget && onCerrar()}>
      <div className="w-full max-w-sm rounded-xl border border-border bg-surface-raised p-5 shadow-lifted">
        <h2 className="mb-1 text-base font-semibold text-content">Restablecer contraseña</h2>
        <p className="mb-3 text-xs text-content-muted">{usuario.nombre} · {usuario.email}</p>
        <Input
          label="Nueva contraseña temporal"
          value={password}
          onChange={(e) => setPassword(e.target.value)}
          placeholder="Mínimo 8 caracteres"
          autoFocus
        />
        <div className="mt-4 flex justify-end gap-2">
          <Button variant="secondary" onClick={onCerrar}>Cancelar</Button>
          <Button loading={m.isPending} disabled={password.length < 8} onClick={() => m.mutate()}>
            Restablecer
          </Button>
        </div>
      </div>
    </div>,
    document.body,
  );
}
