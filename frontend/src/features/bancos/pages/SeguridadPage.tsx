/**
 * Pantalla — Seguridad (/seguridad): matriz RBAC configurable (permiso × rol),
 * por empresa. Editar requiere el permiso admin.roles (el nav ya la oculta si no).
 * ADMIN es superusuario: su columna va marcada y bloqueada (no se edita).
 * Cada cambio se guarda al instante (PUT del conjunto de permisos del rol) y el
 * backend lo aplica casi en vivo (caché corto por empresa).
 */

import { Fragment, useMemo, useState, type FormEvent } from "react";
import { useQuery, useQueryClient } from "@tanstack/react-query";
import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
  LoadingState,
  PageHeader,
  useToast,
} from "@/components/ui";
import { cn } from "@/lib/cn";
import { mensajeError } from "@/lib/apiError";
import { useAuth } from "@/features/auth/AuthContext";
import { rbacApi, type PermisoDef, type RolItem } from "@/api/rbac";

export function SeguridadPage() {
  const toast = useToast();
  const qc = useQueryClient();
  const { empresaActiva } = useAuth();
  const empId = empresaActiva?.id ?? "none";

  const catQ = useQuery({ queryKey: ["rbac", "catalogo"], queryFn: () => rbacApi.catalogo(), staleTime: 5 * 60_000 });
  const rolesQ = useQuery({ queryKey: ["rbac", "roles", empId], queryFn: () => rbacApi.roles() });
  const matrizQ = useQuery({ queryKey: ["rbac", "matriz", empId], queryFn: () => rbacApi.matriz() });

  const [guardando, setGuardando] = useState<string | null>(null); // rol en guardado
  const [nombreRol, setNombreRol] = useState("");
  const [creando, setCreando] = useState(false);

  const permisos = catQ.data?.permisos ?? [];
  const roles = rolesQ.data ?? [];
  const grants = matrizQ.data ?? [];

  // Set de "rol|permiso" concedidos, para O(1) en el render.
  const concedido = useMemo(() => {
    const s = new Set<string>();
    for (const g of grants) s.add(g.rol_codigo + "|" + g.permiso_codigo);
    return s;
  }, [grants]);

  const grupos = useMemo(() => {
    const m = new Map<string, PermisoDef[]>();
    for (const p of permisos) {
      const arr = m.get(p.Modulo) ?? [];
      arr.push(p);
      m.set(p.Modulo, arr);
    }
    return [...m.entries()];
  }, [permisos]);

  function permisosDeRol(rol: RolItem): string[] {
    return permisos.filter((p) => concedido.has(rol.codigo + "|" + p.Codigo)).map((p) => p.Codigo);
  }

  async function toggle(rol: RolItem, permiso: string, on: boolean) {
    if (rol.es_admin) return;
    const actuales = new Set(permisosDeRol(rol));
    if (on) actuales.add(permiso);
    else actuales.delete(permiso);
    setGuardando(rol.codigo);
    try {
      await rbacApi.setPermisos(rol.codigo, [...actuales]);
      await qc.invalidateQueries({ queryKey: ["rbac", "matriz", empId] });
      // Si el usuario editó SU propio rol, refrescar sus permisos efectivos.
      await qc.invalidateQueries({ queryKey: ["rbac", "mis-permisos"] });
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setGuardando(null);
    }
  }

  async function crearRol(e: FormEvent) {
    e.preventDefault();
    const n = nombreRol.trim();
    if (!n) return;
    setCreando(true);
    try {
      await rbacApi.crearRol(n);
      await qc.invalidateQueries({ queryKey: ["rbac", "roles", empId] });
      await qc.invalidateQueries({ queryKey: ["rbac", "matriz", empId] });
      setNombreRol("");
      toast.success(`Rol «${n}» creado — marcá sus permisos.`);
    } catch (err) {
      toast.error(mensajeError(err));
    } finally {
      setCreando(false);
    }
  }

  const cargando = catQ.isPending || rolesQ.isPending || matrizQ.isPending;
  const error = catQ.error ?? rolesQ.error ?? matrizQ.error;

  return (
    <div className="flex flex-col gap-5">
      <PageHeader
        title="Seguridad · Roles y permisos"
        description={`Matriz de acceso de ${empresaActiva?.nombre ?? "la empresa"}. Los cambios aplican de inmediato.`}
      />

      {cargando ? (
        <LoadingState label="Cargando matriz de permisos" />
      ) : error ? (
        <ErrorState message={mensajeError(error)} onRetry={() => { void catQ.refetch(); void rolesQ.refetch(); void matrizQ.refetch(); }} />
      ) : (
        <Card>
          <CardContent className="overflow-x-auto p-0">
            <table className="w-full border-collapse text-sm">
              <thead>
                <tr>
                  <th className="sticky left-0 z-10 min-w-[280px] bg-surface-raised px-4 py-3 text-left text-[11px] font-bold uppercase tracking-wider text-content-muted">
                    Permiso
                  </th>
                  {roles.map((r) => (
                    <th key={r.codigo} className="border-b-2 border-border px-2 py-2.5 align-bottom">
                      <div className="text-xs font-bold leading-tight">{r.nombre}</div>
                      <div className="mt-0.5 text-[10px] font-normal text-content-muted">
                        {r.es_admin ? "Superusuario" : r.es_base ? "Base" : "A medida"}
                        {guardando === r.codigo && <span className="ml-1 text-accent">· guardando…</span>}
                      </div>
                    </th>
                  ))}
                </tr>
              </thead>
              <tbody>
                {grupos.map(([modulo, perms]) => (
                  <Fragment key={modulo}>
                    <tr>
                      <td
                        colSpan={roles.length + 1}
                        className="sticky left-0 bg-surface-muted px-4 py-1.5 text-[11px] font-extrabold uppercase tracking-wider text-content-muted"
                      >
                        {modulo}
                      </td>
                    </tr>
                    {perms.map((p) => (
                      <tr key={p.Codigo} className="border-t border-border hover:bg-accent/5">
                        <td className="sticky left-0 z-10 bg-surface-raised px-4 py-2">
                          <div className="font-medium">
                            {p.Nombre}
                            {p.Critico && (
                              <span className="ml-1.5 rounded-full border border-negativo px-1.5 py-px text-[9px] font-extrabold tracking-wide text-negativo">
                                CRÍTICO
                              </span>
                            )}
                          </div>
                          <div className="text-[11px] text-content-muted">{p.Descripcion}</div>
                        </td>
                        {roles.map((r) => {
                          const on = r.es_admin || concedido.has(r.codigo + "|" + p.Codigo);
                          return (
                            <td key={r.codigo} className="px-2 py-2 text-center">
                              <input
                                type="checkbox"
                                checked={on}
                                disabled={r.es_admin || guardando === r.codigo}
                                onChange={(e) => void toggle(r, p.Codigo, e.target.checked)}
                                aria-label={`${p.Nombre} para ${r.nombre}`}
                                className={cn(
                                  "h-[18px] w-[18px] rounded border-border",
                                  r.es_admin ? "accent-brand-gold" : "accent-accent",
                                  r.es_admin && "cursor-not-allowed opacity-70",
                                )}
                                title={r.es_admin ? "ADMIN es superusuario: acceso total (no editable)" : undefined}
                              />
                            </td>
                          );
                        })}
                      </tr>
                    ))}
                  </Fragment>
                ))}
              </tbody>
              <tfoot>
                <tr className="border-t-2 border-border">
                  <td className="sticky left-0 bg-surface-raised px-4 py-2 text-[10.5px] font-bold uppercase tracking-wide text-content-muted">
                    Total permisos
                  </td>
                  {roles.map((r) => (
                    <td key={r.codigo} className="px-2 py-2 text-center text-xs font-semibold text-content-muted">
                      {r.es_admin ? permisos.length : permisosDeRol(r).length}/{permisos.length}
                    </td>
                  ))}
                </tr>
              </tfoot>
            </table>
          </CardContent>
        </Card>
      )}

      <Card>
        <CardHeader>
          <CardTitle>Crear rol a medida</CardTitle>
        </CardHeader>
        <CardContent>
          <form onSubmit={crearRol} className="flex flex-wrap items-end gap-3">
            <Input
              label="Nombre del rol"
              value={nombreRol}
              onChange={(e) => setNombreRol(e.target.value)}
              placeholder="Ej. Tesorería, Solo Bancos"
              className="min-w-56"
            />
            <Button type="submit" loading={creando} disabled={!nombreRol.trim()}>
              Crear rol
            </Button>
          </form>
          <p className="mt-2 text-xs text-content-muted">
            Nace con lo mínimo (ver Bancos y CxP); luego marcá sus permisos en la matriz. Los cambios de la
            matriz aplican a esta empresa; cada empresa tiene su propia configuración.
          </p>
        </CardContent>
      </Card>
    </div>
  );
}
