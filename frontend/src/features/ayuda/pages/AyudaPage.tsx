/**
 * Ayuda (/ayuda) — el manual, adentro del sistema.
 *
 * EL PEDIDO (punto 3 de la revisión comercial): «No hay un manual de "cómo cierro el mes". Eso es
 * capacitación que hoy das vos en persona, y es lo que no escala».
 *
 * ESTÁ ORDENADO POR LO QUE HAY QUE HACER, no por módulo. Alguien que entra al sistema no busca
 * «Bancos»: busca «cómo cierro el mes», «cómo le pago a un proveedor», «por qué no puedo entrar».
 *
 * Se puede IMPRIMIR, y eso no es un adorno: hay gente que aprende con el papel al lado del
 * teclado, y mandarle el manual a alguien que todavía no tiene usuario es un caso real. Es el
 * mismo camino que el resto de los papeles del sistema (window.print, sin generar un PDF aparte
 * que después nadie regenera cuando el texto cambia).
 */

import { useMemo, useState } from "react";
import { useSearchParams } from "react-router-dom";

import {
  Button,
  Card,
  CardContent,
  CardHeader,
  CardTitle,
  EmptyState,
  Input,
  PageHeader,
} from "@/components/ui";
import { buscarGuias, GUIAS } from "@/features/ayuda/guias";
import type { Bloque, Guia } from "@/features/ayuda/tipos";

export function AyudaPage() {
  const [params, setParams] = useSearchParams();
  const [q, setQ] = useState("");

  // La guía abierta viaja en la URL: así el enlace «Ver la guía completa» de la ayuda contextual
  // aterriza en el lugar exacto, y la dirección se puede pasar a alguien.
  const abierta = params.get("guia") ?? "";
  const encontradas = useMemo(() => buscarGuias(q), [q]);

  function abrir(id: string) {
    const next = new URLSearchParams(params);
    if (abierta === id) next.delete("guia");
    else next.set("guia", id);
    setParams(next, { replace: true });
  }

  return (
    <div className="flex flex-col gap-6">
      <PageHeader
        title="Ayuda"
        description="Cómo se hace cada cosa en el sistema, ordenado por lo que hay que hacer."
        actions={
          <Button variant="secondary" className="print:hidden" onClick={() => window.print()}>
            Imprimir el manual
          </Button>
        }
      />

      {/* El buscador recorre TODO el texto de las guías, no solo los títulos: quien busca
          «macro» o «rebotada» no sabe en qué guía está esa palabra, y por eso busca. */}
      <Card className="print:hidden">
        <CardContent className="pt-5">
          <Input
            label="Buscar en el manual"
            value={q}
            onChange={(e) => setQ(e.target.value)}
            placeholder="Ej.: macro, nota de crédito, no puedo entrar…"
          />
        </CardContent>
      </Card>

      {encontradas.length === 0 ? (
        <EmptyState message={`No hay nada sobre «${q}». Probá con otra palabra: el buscador mira el texto completo.`} />
      ) : (
        <div className="flex flex-col gap-3 print:hidden">
          {encontradas.map((g) => (
            <TarjetaGuia
              key={g.id}
              g={g}
              // Al buscar se abren todas: lo que se quiere ver es DÓNDE aparece la palabra, y para
              // eso hay que leer el contenido, no el título.
              abierta={abierta === g.id || q.trim() !== ""}
              onAbrir={() => abrir(g.id)}
            />
          ))}
        </div>
      )}

      {/* Al imprimir se lleva TODO, no lo que esté abierto en pantalla: quien imprime quiere el
          manual, no el estado de su navegador. */}
      <div className="hidden print:block">
        {GUIAS.map((g) => (
          <section key={g.id} className="mb-6 break-inside-avoid">
            <h2 className="text-lg font-semibold text-content">{g.titulo}</h2>
            <p className="mb-2 text-sm text-content-muted">{g.resumen}</p>
            <Contenido bloques={g.bloques} />
          </section>
        ))}
      </div>
    </div>
  );
}

function TarjetaGuia({ g, abierta, onAbrir }: { g: Guia; abierta: boolean; onAbrir: () => void }) {
  return (
    <Card className="print:hidden">
      <CardHeader>
        <button type="button" onClick={onAbrir} className="w-full text-left">
          <CardTitle>{g.titulo}</CardTitle>
          <p className="mt-1 text-sm font-normal text-content-muted">{g.resumen}</p>
        </button>
      </CardHeader>
      {abierta && (
        <CardContent>
          <Contenido bloques={g.bloques} />
        </CardContent>
      )}
    </Card>
  );
}

/** Dibuja los bloques. Cada tipo tiene su forma; no hay un markdown que interpretar. */
function Contenido({ bloques }: { bloques: Bloque[] }) {
  return (
    <div className="flex flex-col gap-3">
      {bloques.map((b, i) => (
        <BloqueVista key={i} b={b} />
      ))}
    </div>
  );
}

function BloqueVista({ b }: { b: Bloque }) {
  switch (b.tipo) {
    case "texto":
      return <p className="text-sm leading-relaxed text-content">{b.texto}</p>;

    case "aviso":
      // Lo que conviene saber ANTES de que pase. Se destaca porque es lo que, si no se lee,
      // termina en una llamada.
      return (
        <div className="rounded-lg border border-pendiente/40 bg-pendiente/5 px-3 py-2 text-sm leading-relaxed text-content">
          {b.texto}
        </div>
      );

    case "lista":
      return (
        <ul className="flex flex-col gap-2">
          {b.items.map((it, i) => (
            <li key={i} className="flex gap-2 text-sm leading-relaxed text-content">
              <span className="mt-2 h-1 w-1 shrink-0 rounded-full bg-accent" />
              <span>{it}</span>
            </li>
          ))}
        </ul>
      );

    case "pasos":
      return (
        <ol className="flex flex-col gap-3">
          {b.items.map((p, i) => (
            <li key={i} className="flex gap-3">
              <span className="mt-0.5 flex h-6 w-6 shrink-0 items-center justify-center rounded-full bg-accent/15 text-xs font-semibold text-accent">
                {i + 1}
              </span>
              <div>
                <p className="text-sm font-medium text-content">
                  {p.titulo}
                  {p.donde && <span className="ml-2 text-xs font-normal text-accent">{p.donde}</span>}
                </p>
                <p className="mt-0.5 text-sm leading-relaxed text-content-muted">{p.texto}</p>
              </div>
            </li>
          ))}
        </ol>
      );

    case "tabla":
      return (
        <div className="overflow-x-auto rounded-lg border border-border">
          <table className="w-full text-sm">
            <thead>
              <tr className="bg-surface-raised">
                {b.encabezados.map((h, i) => (
                  <th key={i} className="px-3 py-2 text-left font-medium text-content">{h}</th>
                ))}
              </tr>
            </thead>
            <tbody>
              {b.filas.map((f, i) => (
                <tr key={i} className="border-t border-border align-top">
                  {f.map((c, j) => (
                    <td key={j} className="px-3 py-2 leading-relaxed text-content-muted">{c}</td>
                  ))}
                </tr>
              ))}
            </tbody>
          </table>
        </div>
      );

    case "flujo":
      return (
        <div className="flex flex-wrap items-center gap-2">
          {b.etapas.map((e, i) => (
            <span key={i} className="flex items-center gap-2">
              <span className="rounded-lg border border-border bg-surface-raised px-2.5 py-1 text-xs font-medium text-content">
                {e}
              </span>
              {i < b.etapas.length - 1 && <span className="text-content-muted">→</span>}
            </span>
          ))}
        </div>
      );

    case "formula":
      // Monoespaciada y respetando los espacios: la alineación de la resta ES la explicación.
      return (
        <pre className="overflow-x-auto rounded-lg border border-border bg-surface p-3 font-mono text-xs leading-relaxed text-content">
          {b.lineas.join("\n")}
        </pre>
      );
  }
}
