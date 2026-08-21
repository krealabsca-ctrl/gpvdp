package seed

import "testing"

// El seed de demo NO puede volver a pisar el catálogo del usuario.
//
// Lo que pasó en la base real: el usuario fusionó «Depósitos de Clientes» hacia su propia
// «Deposito de Clientes» (sin tilde) a las 17:27, y la clasificación reapareció a las 18:01 con id
// nuevo y sin evento de auditoría — en el siguiente reinicio del backend. El `ON CONFLICT DO
// NOTHING` no protegía nada porque calza por nombre EXACTO: el seed no reconoce el nombre que el
// usuario eligió.
//
// Este test no puede correr SQL, así que fija las dos propiedades del código que hacían daño y que
// una edición distraída podría reintroducir:
//  1. `seedCatalogo` arranca con una guarda: si la empresa ya tiene conceptos, no toca nada.
//  2. El upsert del concepto NO reactiva (`activo = true`) lo que el usuario desactivó.
func TestSeedCatalogoNoRevivaElCatalogoDelUsuario(t *testing.T) {
	t.Parallel()

	fuente := codigoDeSeedCatalogo(t)

	// 1) La guarda: sin ella el seed vuelve a insertar en cada arranque.
	if !contiene(fuente, "SELECT EXISTS (SELECT 1 FROM concepto WHERE empresa_id") {
		t.Error("seedCatalogo perdió la guarda «¿la empresa ya tiene catálogo?»: volvería a pisar el trabajo del usuario en cada reinicio")
	}
	if !contiene(fuente, "if tiene {") {
		t.Error("la guarda no corta la ejecución: leer el estado y seguir igual no protege nada")
	}

	// 2) Nunca reactivar: `activo = true` en el upsert del concepto revivía lo desactivado.
	if contiene(fuente, "SET activo = true") {
		t.Error("el upsert del concepto volvió a reactivar lo desactivado (SET activo = true)")
	}
}

// El catálogo demo se mantiene chico y con nombres estables: cada nombre de acá es un nombre que el
// usuario puede llegar a fusionar o renombrar, y con la guarda puesta ya no se recrea.
func TestCatalogoDemoSinDuplicados(t *testing.T) {
	t.Parallel()
	vistos := map[string]bool{}
	for _, cd := range conceptosDemo {
		if vistos[cd.Concepto] {
			t.Errorf("concepto demo duplicado: %q", cd.Concepto)
		}
		vistos[cd.Concepto] = true

		enConcepto := map[string]bool{}
		for _, cl := range cd.Clasificaciones {
			if enConcepto[cl] {
				t.Errorf("clasificación demo duplicada en %q: %q", cd.Concepto, cl)
			}
			enConcepto[cl] = true
		}
	}
}
