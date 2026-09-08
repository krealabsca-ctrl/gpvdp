/**
 * Administrar el catálogo de departamentos desde la pantalla donde se usa.
 *
 * ── Las dos cosas que la pantalla tiene que dejar claras ─────────────────────
 *
 *  1. **Es el MISMO catálogo que usa CxP.** Una sola tabla: lo que se crea acá aparece en las
 *     facturas, en los empleados y en los fondos de caja chica. Se dice arriba, porque «crear
 *     departamento» en una pantalla de Bancos hace pensar que es una lista aparte de Bancos.
 *
 *  2. **Eliminar y desactivar no son lo mismo.** Se borra de verdad solo lo que no tiene nada
 *     colgando; en cuanto tiene historia, el camino es desactivar —deja de ofrecerse para elegir,
 *     pero los movimientos ya atribuidos siguen diciendo a quién se le cobraron—. El botón de
 *     eliminar no aparece cuando no se puede: se consulta el uso antes de ofrecerlo.
 */

import { useState } from "react";
import {
  Badge,
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  ConfirmDialog,
  Input,
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
import { useTienePermiso } from "@/features/auth/permisos";
import {
  useCambiarActivoDepartamento,
  useCrearDepartamento,
  useDepartamentosDeBancos,
  useEliminarDepartamento,
  useRenombrarDepartamento,
  useSedes,
  useUsoDeDepartamento,
} from "@/features/bancos/hooks";
import type { DepartamentoRef } from "@/api/bancos";

export function AdminDepartamentos() {
  const toast = useToast();
  const tienePermiso = useTienePermiso();
  const puedeEditar = tienePermiso("bancos.catalogo");

  const deptos = useDepartamentosDeBancos();
  const sedes = useSedes();
  const crear = useCrearDepartamento();

  const [nuevo, setNuevo] = useState("");
  const [codigo, setCodigo] = useState("");
  const [editando, setEditando] = useState<string | null>(null);
  const [porEliminar, setPorEliminar] = useState<DepartamentoRef | null>(null);

  function agregar() {
    if (nuevo.trim() === "") {
      toast.error("Escribí el nombre del departamento.");
      return;
    }
    crear.mutate(
      { nombre: nuevo.trim(), codigo: codigo.trim() },
      {
        onSuccess: (d) => {
          toast.success(`«${d.nombre}» quedó en el catálogo, y también lo ve CxP.`);
          setNuevo("");
          setCodigo("");
        },
        onError: (err) => toast.error(mensajeError(err)),
      },
    );
  }

  const lista = deptos.data ?? [];

  return (
    <Card>
      <CardHeader>
        <CardTitle>Departamentos y sedes</CardTitle>
        <p className="mt-1 text-xs text-content-muted">
          Es el <strong>mismo catálogo</strong> que usan las facturas de CxP, los empleados y los
          fondos de caja chica: no hay dos listas que puedan contradecirse. El departamento dice
          QUIÉN gastó; la sede, DÓNDE.
        </p>
      </CardHeader>
      <CardContent className="flex flex-col gap-4">
        {/*
          Alineado por ARRIBA y sin textos de ayuda debajo de los campos: un hint («opcional») deja
          el campo que lo tiene más alto que los demás y el botón flotando, que es lo que se veía
          torcido. Lo que el hint decía ahora vive en el propio label.
        */}
        {puedeEditar && (
          <div className="flex flex-wrap items-start gap-2">
            <div className="w-56">
              <Input
                label="Departamento nuevo"
                value={nuevo}
                onChange={(e) => setNuevo(e.target.value)}
                placeholder="Logística"
              />
            </div>
            <div className="w-32">
              <Input
                label="Código (opcional)"
                value={codigo}
                onChange={(e) => setCodigo(e.target.value)}
                placeholder="LOG"
              />
            </div>
            <div className="flex flex-col gap-1.5">
              <span className="text-sm font-medium text-transparent" aria-hidden="true">
                &nbsp;
              </span>
              <Button onClick={agregar} loading={crear.isPending}>
                Agregar
              </Button>
            </div>
          </div>
        )}
        {!puedeEditar && (
          <p className="text-sm text-content-muted">
            Para crear o cambiar departamentos hace falta el permiso <code>bancos.catalogo</code>.
          </p>
        )}

        <TableContainer>
          <Table>
            <THead>
              <TR>
                <TH>Departamento</TH>
                <TH>Código</TH>
                <TH>Estado</TH>
                {puedeEditar && <TH className="text-right">Acciones</TH>}
              </TR>
            </THead>
            <TBody>
              {lista.map((d) =>
                editando === d.id ? (
                  <FilaEnEdicion
                    key={d.id}
                    depto={d}
                    onCerrar={() => setEditando(null)}
                    onError={(m) => toast.error(m)}
                    onHecho={(m) => {
                      toast.success(m);
                      setEditando(null);
                    }}
                  />
                ) : (
                  <TR key={d.id}>
                    <TD className="font-medium text-content">{d.nombre}</TD>
                    <TD className="text-content-muted">{d.codigo || "—"}</TD>
                    <TD>
                      <Badge tone={d.activo ? "positivo" : "neutral"}>
                        {d.activo ? "activo" : "inactivo"}
                      </Badge>
                    </TD>
                    {puedeEditar && (
                      <TD className="text-right">
                        <div className="flex justify-end gap-2">
                          <Button size="sm" variant="ghost" onClick={() => setEditando(d.id)}>
                            Renombrar
                          </Button>
                          <BotonActivo
                            depto={d}
                            onHecho={(m) => toast.success(m)}
                            onError={(m) => toast.error(m)}
                          />
                          <Button
                            size="sm"
                            variant="ghost"
                            onClick={() => setPorEliminar(d)}
                            className="text-negativo"
                          >
                            Eliminar
                          </Button>
                        </div>
                      </TD>
                    )}
                  </TR>
                ),
              )}
              {lista.length === 0 && (
                <TR>
                  <TD colSpan={puedeEditar ? 4 : 3} className="text-content-muted">
                    Todavía no hay departamentos en el catálogo.
                  </TD>
                </TR>
              )}
            </TBody>
          </Table>
        </TableContainer>

        <p className="text-xs text-content-muted">
          Sedes cargadas: {(sedes.data ?? []).length}. Se administran en Catálogo › Sedes; acá se
          eligen al atribuir una partida.
        </p>
      </CardContent>

      {porEliminar && (
        <DialogoEliminar depto={porEliminar} onCerrar={() => setPorEliminar(null)} />
      )}
    </Card>
  );
}

/** La fila mientras se renombra. */
function FilaEnEdicion({
  depto,
  onCerrar,
  onHecho,
  onError,
}: {
  depto: DepartamentoRef;
  onCerrar: () => void;
  onHecho: (msg: string) => void;
  onError: (msg: string) => void;
}) {
  const renombrar = useRenombrarDepartamento();
  const [nombre, setNombre] = useState(depto.nombre);
  const [codigo, setCodigo] = useState(depto.codigo);

  function guardar() {
    if (nombre.trim() === "") {
      onError("El nombre no puede quedar vacío.");
      return;
    }
    renombrar.mutate(
      { id: depto.id, nombre: nombre.trim(), codigo: codigo.trim() },
      {
        onSuccess: () => onHecho(`Quedó como «${nombre.trim()}».`),
        onError: (err) => onError(mensajeError(err)),
      },
    );
  }

  return (
    <TR className="bg-surface-muted">
      <TD>
        <Input
          aria-label="Nombre del departamento"
          value={nombre}
          onChange={(e) => setNombre(e.target.value)}
        />
      </TD>
      <TD>
        <Input aria-label="Código" value={codigo} onChange={(e) => setCodigo(e.target.value)} />
      </TD>
      <TD className="text-xs text-content-muted">
        el nombre cambia también en CxP y en la historia ya atribuida
      </TD>
      <TD className="text-right">
        <div className="flex justify-end gap-2">
          <Button size="sm" onClick={guardar} loading={renombrar.isPending}>
            Guardar
          </Button>
          <Button size="sm" variant="ghost" onClick={onCerrar}>
            Cancelar
          </Button>
        </div>
      </TD>
    </TR>
  );
}

function BotonActivo({
  depto,
  onHecho,
  onError,
}: {
  depto: DepartamentoRef;
  onHecho: (msg: string) => void;
  onError: (msg: string) => void;
}) {
  const cambiar = useCambiarActivoDepartamento();
  return (
    <Button
      size="sm"
      variant="ghost"
      loading={cambiar.isPending}
      onClick={() =>
        cambiar.mutate(
          { id: depto.id, activo: !depto.activo },
          {
            onSuccess: () =>
              onHecho(
                depto.activo
                  ? `«${depto.nombre}» ya no se ofrece para elegir; su historia queda intacta.`
                  : `«${depto.nombre}» vuelve a estar disponible.`,
              ),
            onError: (err) => onError(mensajeError(err)),
          },
        )
      }
    >
      {depto.activo ? "Desactivar" : "Reactivar"}
    </Button>
  );
}

/**
 * El diálogo de eliminar, que primero pregunta al servidor de qué cuelga.
 *
 * Ofrecer «eliminar» y contestar después «no se puede» es la peor versión: el usuario ya decidió.
 * Acá el uso se consulta al abrir, y si hay historia el diálogo directamente propone desactivar.
 */
function DialogoEliminar({
  depto,
  onCerrar,
}: {
  depto: DepartamentoRef;
  onCerrar: () => void;
}) {
  const toast = useToast();
  const uso = useUsoDeDepartamento(depto.id, true);
  const eliminar = useEliminarDepartamento();
  const cambiar = useCambiarActivoDepartamento();

  if (uso.isLoading) {
    return (
      <ConfirmDialog
        titulo={`Eliminar «${depto.nombre}»`}
        descripcion="Revisando de qué cuelga este departamento…"
        pendiente
        onConfirmar={() => undefined}
        onCancelar={onCerrar}
      />
    );
  }

  const sePuede = uso.data?.se_puede_eliminar ?? false;

  if (!sePuede) {
    return (
      <ConfirmDialog
        titulo={`«${depto.nombre}» no se puede eliminar`}
        descripcion={
          <>
            Tiene <strong>{uso.data?.detalle}</strong> colgando. Borrarlo dejaría esa historia sin
            dueño, así que la salida es <strong>desactivarlo</strong>: deja de ofrecerse para elegir
            y lo ya atribuido sigue diciendo a quién se le cobró.
          </>
        }
        textoConfirmar={depto.activo ? "Desactivar" : "Entendido"}
        tono="peligro"
        pendiente={cambiar.isPending}
        onConfirmar={() => {
          if (!depto.activo) {
            onCerrar();
            return;
          }
          cambiar.mutate(
            { id: depto.id, activo: false },
            {
              onSuccess: () => {
                toast.success(`«${depto.nombre}» quedó inactivo.`);
                onCerrar();
              },
              onError: (err) => toast.error(mensajeError(err)),
            },
          );
        }}
        onCancelar={onCerrar}
      />
    );
  }

  return (
    <ConfirmDialog
      titulo={`Eliminar «${depto.nombre}»`}
      descripcion="No tiene nada colgando, así que se borra del catálogo de verdad. Esto no se puede deshacer."
      impacto={["También desaparece del catálogo de CxP, RRHH y caja chica."]}
      textoConfirmar="Eliminar"
      tono="peligro"
      pendiente={eliminar.isPending}
      onConfirmar={() =>
        eliminar.mutate(depto.id, {
          onSuccess: () => {
            toast.success(`«${depto.nombre}» se eliminó.`);
            onCerrar();
          },
          onError: (err) => toast.error(mensajeError(err)),
        })
      }
      onCancelar={onCerrar}
    />
  );
}
