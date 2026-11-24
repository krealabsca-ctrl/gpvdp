/**
 * «Prioridades de pago»: el catálogo que decide qué se paga antes (mig 0121).
 *
 * «Que se pueda agregar nuevas prioridades.» Hasta la 0121 eran tres valores escritos en la base,
 * así que una cuarta exigía una migración y un despliegue.
 *
 * LAS DOS COLUMNAS QUE PARECEN LO MISMO Y NO LO SON:
 *
 *   Orden    manda en cómo se ordenan las listas (mayor = primero).
 *   Urgente  dice si cuenta en «lo que se paga sí o sí»: el tablero, el filtro y la premarca del
 *            corte.
 *
 * Se declaran por separado a propósito. Si la urgencia se dedujera del orden —«la más alta es la
 * urgente»— crear una prioridad nueva por encima dejaría de contar la anterior en silencio, y el
 * tablero mostraría menos cartera urgente sin que nadie tocara una factura.
 */
import { useState } from "react";

import {
  Badge,
  Button,
  Card,
  CardContent,
  CardDescription,
  CardHeader,
  CardTitle,
  ErrorState,
  Input,
  LoadingState,
  Select,
  Table,
  TableContainer,
  TBody,
  TD,
  TH,
  THead,
  TR,
  useToast,
} from "@/components/ui";
import type { Prioridad, PrioridadInput, TonoPrioridad } from "@/api/cxp";
import {
  useActualizarPrioridad,
  useCrearPrioridad,
  useEliminarPrioridad,
  usePrioridades,
} from "@/features/cxp/hooks";
import { mensajeError } from "@/lib/apiError";

const TONOS: { value: TonoPrioridad; label: string }[] = [
  { value: "negativo", label: "Rojo — urgente" },
  { value: "pendiente", label: "Ámbar — atención" },
  { value: "accent", label: "Azul — informativo" },
  { value: "positivo", label: "Verde — al día" },
  { value: "neutral", label: "Gris — sin énfasis" },
];

const VACIO: PrioridadInput = {
  codigo: "",
  nombre: "",
  orden: 0,
  urgente: false,
  tono: "pendiente",
  activa: true,
};

export function PrioridadesPanel() {
  const toast = useToast();
  const q = usePrioridades();
  const crear = useCrearPrioridad();
  const actualizar = useActualizarPrioridad();
  const eliminar = useEliminarPrioridad();

  const [nueva, setNueva] = useState<PrioridadInput>(VACIO);
  const [editando, setEditando] = useState<string | null>(null);
  const [edicion, setEdicion] = useState<PrioridadInput>(VACIO);

  const items = q.data?.items ?? [];

  function alCrear() {
    crear.mutate(nueva, {
      onSuccess: (p) => {
        toast.success(`Prioridad ${p.codigo} creada`);
        setNueva(VACIO);
      },
      onError: (e) => toast.error(mensajeError(e)),
    });
  }

  function empezarAEditar(p: Prioridad) {
    setEditando(p.id);
    setEdicion({
      codigo: p.codigo,
      nombre: p.nombre,
      orden: p.orden,
      urgente: p.urgente,
      tono: p.tono,
      activa: p.activa,
    });
  }

  function guardar(id: string) {
    actualizar.mutate(
      { id, input: edicion },
      {
        onSuccess: () => {
          toast.success("Prioridad actualizada");
          setEditando(null);
        },
        onError: (e) => toast.error(mensajeError(e)),
      },
    );
  }

  return (
    <Card>
      <CardHeader>
        <CardTitle>Prioridades de pago</CardTitle>
        <CardDescription>
          Deciden qué se paga antes. <strong>Orden</strong> manda en cómo se ordenan las listas (el
          número más alto va primero) y <strong>urgente</strong> dice si cuenta en «lo que se paga
          sí o sí»: el tablero, el filtro y la premarca del corte. Son cosas distintas a propósito —
          crear una prioridad por encima no debería hacer que la anterior deje de contar.
        </CardDescription>
      </CardHeader>
      <CardContent className="space-y-4">
        {q.isPending && <LoadingState label="Cargando prioridades" />}
        {q.isError && <ErrorState message={mensajeError(q.error)} />}

        {!q.isPending && !q.isError && (
          <TableContainer>
            <Table>
              <THead>
                <TR>
                  <TH>Código</TH>
                  <TH>Nombre</TH>
                  <TH className="text-right">Orden</TH>
                  <TH>Urgente</TH>
                  <TH>Color</TH>
                  <TH>Estado</TH>
                  <TH className="text-right">En uso</TH>
                  <TH />
                </TR>
              </THead>
              <TBody>
                {items.map((p) =>
                  editando === p.id ? (
                    <TR key={p.id}>
                      {/* El CÓDIGO no se edita: está escrito en las facturas que ya lo llevan, y
                          cambiarlo las dejaría apuntando a uno que no existe. */}
                      <TD className="font-medium">{p.codigo}</TD>
                      <TD>
                        <Input
                          value={edicion.nombre}
                          onChange={(e) => setEdicion({ ...edicion, nombre: e.target.value })}
                          aria-label={`Nombre de ${p.codigo}`}
                        />
                      </TD>
                      <TD>
                        <Input
                          type="number"
                          value={String(edicion.orden)}
                          onChange={(e) =>
                            setEdicion({ ...edicion, orden: Number(e.target.value) || 0 })
                          }
                          className="w-20 text-right"
                          aria-label={`Orden de ${p.codigo}`}
                        />
                      </TD>
                      <TD>
                        <input
                          type="checkbox"
                          checked={edicion.urgente}
                          onChange={(e) => setEdicion({ ...edicion, urgente: e.target.checked })}
                          aria-label={`${p.codigo} cuenta como urgente`}
                        />
                      </TD>
                      <TD>
                        <Select
                          value={edicion.tono}
                          onChange={(e) =>
                            setEdicion({ ...edicion, tono: e.target.value as TonoPrioridad })
                          }
                          options={TONOS}
                          aria-label={`Color de ${p.codigo}`}
                        />
                      </TD>
                      <TD>
                        <input
                          type="checkbox"
                          checked={edicion.activa}
                          onChange={(e) => setEdicion({ ...edicion, activa: e.target.checked })}
                          aria-label={`${p.codigo} activa`}
                        />
                      </TD>
                      <TD className="text-right tabular-nums">{p.en_uso}</TD>
                      <TD className="text-right">
                        <Button onClick={() => guardar(p.id)} disabled={actualizar.isPending}>
                          Guardar
                        </Button>
                        <Button variant="ghost" onClick={() => setEditando(null)}>
                          Cancelar
                        </Button>
                      </TD>
                    </TR>
                  ) : (
                    <TR key={p.id}>
                      <TD>
                        <Badge tone={p.tono}>{p.codigo}</Badge>
                      </TD>
                      <TD>{p.nombre}</TD>
                      <TD className="text-right tabular-nums">{p.orden}</TD>
                      <TD>
                        {p.urgente ? (
                          <Badge tone="negativo">Sí</Badge>
                        ) : (
                          <span className="text-content-muted">—</span>
                        )}
                      </TD>
                      <TD className="text-content-muted">
                        {TONOS.find((t) => t.value === p.tono)?.label ?? p.tono}
                      </TD>
                      <TD>
                        {p.activa ? (
                          <span className="text-content-muted">Activa</span>
                        ) : (
                          <Badge tone="neutral">Retirada</Badge>
                        )}
                      </TD>
                      <TD className="text-right tabular-nums">{p.en_uso}</TD>
                      <TD className="text-right">
                        <Button variant="ghost" onClick={() => empezarAEditar(p)}>
                          Editar
                        </Button>
                        {/* Eliminar solo se ofrece si no cuelga ninguna factura. Con facturas
                            puestas se desactiva: borrarla las dejaría con un código que ninguna
                            pantalla sabe nombrar ni colorear. */}
                        {p.en_uso === 0 && (
                          <Button
                            variant="ghost"
                            onClick={() =>
                              eliminar.mutate(p.id, {
                                onSuccess: () => toast.success(`Prioridad ${p.codigo} eliminada`),
                                onError: (e) => toast.error(mensajeError(e)),
                              })
                            }
                            disabled={eliminar.isPending}
                          >
                            Eliminar
                          </Button>
                        )}
                      </TD>
                    </TR>
                  ),
                )}
              </TBody>
            </Table>
          </TableContainer>
        )}

        <div className="space-y-3 border-t border-border pt-4">
          <h3 className="text-sm font-medium">Agregar una prioridad</h3>
          <div className="flex flex-wrap items-end gap-3">
            <div>
              <label className="mb-1 block text-xs text-content-muted" htmlFor="prio-codigo">
                Código
              </label>
              <Input
                id="prio-codigo"
                value={nueva.codigo}
                onChange={(e) => setNueva({ ...nueva, codigo: e.target.value.toUpperCase() })}
                placeholder="AAA"
                className="w-24"
              />
            </div>
            <div className="min-w-48 flex-1">
              <label className="mb-1 block text-xs text-content-muted" htmlFor="prio-nombre">
                ¿Cuándo se usa?
              </label>
              <Input
                id="prio-nombre"
                value={nueva.nombre}
                onChange={(e) => setNueva({ ...nueva, nombre: e.target.value })}
                placeholder="Ej.: urgencia absoluta, no puede esperar al corte"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-content-muted" htmlFor="prio-orden">
                Orden
              </label>
              <Input
                id="prio-orden"
                type="number"
                value={String(nueva.orden)}
                onChange={(e) => setNueva({ ...nueva, orden: Number(e.target.value) || 0 })}
                className="w-20 text-right"
              />
            </div>
            <div>
              <label className="mb-1 block text-xs text-content-muted" htmlFor="prio-tono">
                Color
              </label>
              <Select
                id="prio-tono"
                value={nueva.tono}
                onChange={(e) => setNueva({ ...nueva, tono: e.target.value as TonoPrioridad })}
                options={TONOS}
              />
            </div>
            <label className="flex items-center gap-2 pb-2 text-sm">
              <input
                type="checkbox"
                checked={nueva.urgente}
                onChange={(e) => setNueva({ ...nueva, urgente: e.target.checked })}
              />
              Cuenta como urgente
            </label>
            <Button
              onClick={alCrear}
              disabled={crear.isPending || nueva.codigo.trim() === "" || nueva.nombre.trim().length < 3}
            >
              Agregar
            </Button>
          </div>
          <p className="text-xs text-content-muted">
            El orden más alto va primero en las listas. Dejá hueco entre los números (10, 20, 30)
            para poder meter una intermedia después sin renumerar todas.
          </p>
        </div>
      </CardContent>
    </Card>
  );
}
