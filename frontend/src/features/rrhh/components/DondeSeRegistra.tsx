/**
 * «¿Dónde registro esto?» — el mapa del módulo, en la pantalla de entrada de RRHH.
 *
 * Existe porque el usuario reportó que no encontraba dónde usar las horas extra y las vacaciones
 * (2026-08-17). Las dos estaban construidas, pero escondidas: las horas extra viven DENTRO de una
 * corrida —en un bloque que antes se llamaba «Novedades del mes», que no nombra la cosa— y las
 * vacaciones en una pantalla que se llamaba «Ausencias», palabra que nadie usa para buscarlas.
 *
 * La lección: una función que existe pero no se encuentra es, para quien la necesita, una función
 * que no existe. El nombre del menú y el título del bloque son parte de la función.
 */

import { Link } from "react-router-dom";
import { Card, CardContent, CardHeader, CardTitle } from "@/components/ui";

const PASOS: { que: string; donde: string; ruta: string; comoSeHace: string }[] = [
  {
    que: "Horas extra",
    donde: "Corridas → la corrida del período",
    ruta: "/rrhh/corridas",
    comoSeHace:
      "En el bloque «Horas extra, comisiones y bonos». Se anotan las HORAS (no el monto): el sistema paga horas × valor de la hora × 1,5 como mínimo (art. 139 CT).",
  },
  {
    que: "Vacaciones disfrutadas",
    donde: "Vacaciones e incapacidades",
    ruta: "/rrhh/ausencias",
    comoSeHace:
      "Se registran los días que la persona tomó. El saldo de días se calcula solo, y lo disfrutado entra a la corrida del período sin que haya que anotarlo dos veces.",
  },
  {
    que: "Incapacidades (CCSS / INS)",
    donde: "Vacaciones e incapacidades",
    ruta: "/rrhh/ausencias",
    comoSeHace:
      "Con la boleta: el sistema aplica el subsidio de ley y ajusta lo que paga la empresa en la corrida.",
  },
  {
    que: "Comisiones y bonos",
    donde: "Corridas → la corrida del período",
    ruta: "/rrhh/corridas",
    comoSeHace:
      "Mismo bloque que las horas extra, con el monto del período. Son salario: entran a la base de la CCSS por ley.",
  },
  {
    que: "Deducciones fijas (asociación, ahorro, préstamo)",
    donde: "Empleados → la ficha",
    ruta: "/rrhh/empleados",
    comoSeHace:
      "Se cargan una vez con su cuota y su saldo; se descuentan solas cada período y se detienen cuando el saldo llega a cero.",
  },
  {
    que: "Salida de un trabajador",
    donde: "Finiquitos",
    ruta: "/rrhh/finiquitos",
    comoSeHace:
      "Calcula el finiquito conforme al Código de Trabajo y lo incluye en el archivo de pago y en la planilla de la CCSS del mes.",
  },
];

export function DondeSeRegistra() {
  return (
    <Card>
      <CardHeader>
        <CardTitle>¿Dónde registro cada cosa?</CardTitle>
      </CardHeader>
      <CardContent className="flex flex-col gap-2">
        <p className="text-sm text-content-muted">
          El pago del período se arma solo con lo que esté registrado en su lugar. Esta es la guía de
          qué se anota dónde.
        </p>
        <ul className="flex flex-col divide-y divide-border">
          {PASOS.map((p) => (
            <li key={p.que} className="flex flex-col gap-0.5 py-2 sm:flex-row sm:items-baseline sm:gap-3">
              <span className="min-w-56 font-medium text-content">{p.que}</span>
              <span className="flex-1">
                <Link to={p.ruta} className="text-sm font-medium text-accent underline">
                  {p.donde}
                </Link>
                <span className="mt-0.5 block text-xs text-content-muted">{p.comoSeHace}</span>
              </span>
            </li>
          ))}
        </ul>
      </CardContent>
    </Card>
  );
}
