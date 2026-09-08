package inventario

// Reglas de negocio del inventario.
//
// El criterio que guía los avisos de este archivo es el mismo del resto del sistema: **un número que
// puede estar mal se muestra con su advertencia, no se esconde ni se presenta como verdad**. Acá el
// riesgo concreto es que nadie registre los servicios prestados: entonces las existencias solo
// suben, la pantalla se ve sana y en la bodega falta la mitad.

import (
	"context"
	"fmt"
	"strings"

	"github.com/shopspring/decimal"

	"github.com/gpvdp/erp/internal/shared"
)

// umbralCercaDelMinimo es a qué distancia del mínimo se avisa «cerca». Un artículo justo en el
// mínimo ya es tarde para pedir si el proveedor tarda dos semanas.
const umbralCercaDelMinimo = 1.25

// serviciosMinimosParaSugerir es cuántos servicios hacen falta antes de sugerir cantidades de
// pedido. Con menos que esto, el «consumo semanal» sería una cifra inventada con aire de certeza.
const serviciosMinimosParaSugerir = 8

// semanasDeEntrega es el plazo que se asume para cubrir con el pedido. Es un DEFAULT y se escribe en
// la frase que explica cada sugerencia: cuando el negocio confirme el plazo real por proveedor, pasa
// a ser un dato del proveedor y no una constante.
const semanasDeEntrega = 2

// ── Catálogo ────────────────────────────────────────────────────────────────

// Categorias devuelve el árbol de categorías.
func (s *Service) Categorias(ctx context.Context, empresaID string, incluirInactivas bool) ([]Categoria, error) {
	return s.repo.ListarCategorias(ctx, empresaID, incluirInactivas)
}

// CrearCategoria agrega una categoría, opcionalmente hija de otra.
func (s *Service) CrearCategoria(ctx context.Context, empresaID, padreID, nombre, usuarioID string) (Categoria, error) {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return Categoria{}, ErrNombreRequerido
	}
	c, err := s.repo.CrearCategoria(ctx, empresaID, padreID, nombre)
	if err != nil {
		return Categoria{}, err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_categoria", EntidadID: &c.ID,
		Accion: "CREAR_CATEGORIA_INV", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"nombre": nombre, "padre_id": padreID},
	})
	return c, nil
}

// ActualizarCategoria renombra o desactiva una categoría.
func (s *Service) ActualizarCategoria(ctx context.Context, empresaID, id, nombre string, activo bool, usuarioID string) error {
	nombre = strings.TrimSpace(nombre)
	if nombre == "" {
		return ErrNombreRequerido
	}
	if err := s.repo.ActualizarCategoria(ctx, empresaID, id, nombre, activo); err != nil {
		return err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_categoria", EntidadID: &id,
		Accion: "ACTUALIZAR_CATEGORIA_INV", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"nombre": nombre, "activo": activo},
	})
	return nil
}

// Articulos lista el catálogo.
func (s *Service) Articulos(ctx context.Context, empresaID string, f FiltroArticulos) ([]Articulo, error) {
	if f.ModoControl != "" && !modoValido(f.ModoControl) {
		return nil, ErrModoInvalido
	}
	return s.repo.ListarArticulos(ctx, empresaID, f)
}

// CrearArticulo agrega una línea al catálogo.
func (s *Service) CrearArticulo(ctx context.Context, empresaID string, a ArticuloNuevo, usuarioID string) (string, error) {
	if err := validarArticulo(a); err != nil {
		return "", err
	}
	a.Codigo = strings.ToUpper(strings.TrimSpace(a.Codigo))
	a.Nombre = strings.TrimSpace(a.Nombre)
	id, err := s.repo.CrearArticulo(ctx, empresaID, a, usuarioID)
	if err != nil {
		return "", err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_articulo", EntidadID: &id,
		Accion: "CREAR_ARTICULO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"codigo": a.Codigo, "nombre": a.Nombre, "modo": a.ModoControl},
	})
	return id, nil
}

// ActualizarArticulo edita una línea del catálogo.
//
// El modo de control NO se puede cambiar acá: pasar de cantidad a unidad exigiría inventar una ficha
// por cada objeto ya contado, y al revés habría que destruir fichas con historia. Si está mal, se
// crea el artículo correcto y se da de baja el otro.
func (s *Service) ActualizarArticulo(ctx context.Context, empresaID, id string, a ArticuloNuevo, usuarioID string) error {
	if err := validarArticulo(a); err != nil {
		return err
	}
	actual, err := s.repo.ArticuloPorID(ctx, empresaID, id)
	if err != nil {
		return err
	}
	if actual.ModoControl != a.ModoControl {
		return fmt.Errorf("%w: «%s» se controla %s y eso no se cambia con historia encima; creá el artículo correcto y dá de baja este",
			ErrModoInvalido, actual.Nombre, etiquetaModo(actual.ModoControl))
	}
	a.Codigo = strings.ToUpper(strings.TrimSpace(a.Codigo))
	a.Nombre = strings.TrimSpace(a.Nombre)
	if err := s.repo.ActualizarArticulo(ctx, empresaID, id, a); err != nil {
		return err
	}
	// El evento lleva el antes Y el después en el mismo campo: `shared.Evento` no tiene
	// `ValorAnterior`, y sin el valor previo un renombre no se puede reconstruir después.
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_articulo", EntidadID: &id,
		Accion: "ACTUALIZAR_ARTICULO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"antes":   map[string]string{"codigo": actual.Codigo, "nombre": actual.Nombre},
			"despues": map[string]string{"codigo": a.Codigo, "nombre": a.Nombre},
		},
	})
	return nil
}

func etiquetaModo(m string) string {
	if m == ModoUnidad {
		return "por unidad"
	}
	return "por cantidad"
}

func validarArticulo(a ArticuloNuevo) error {
	if strings.TrimSpace(a.Codigo) == "" {
		return ErrCodigoRequerido
	}
	if strings.TrimSpace(a.Nombre) == "" {
		return ErrNombreRequerido
	}
	if a.CategoriaID == "" {
		return ErrCategoriaNoEncontrada
	}
	if !modoValido(a.ModoControl) {
		return ErrModoInvalido
	}
	return nil
}

// FijarNivel define el mínimo y el máximo de un artículo en una sede.
func (s *Service) FijarNivel(ctx context.Context, empresaID, articuloID, sedeID string, minimo, maximo int, usuarioID string) error {
	if sedeID == "" {
		return ErrSedeRequerida
	}
	if minimo < 0 || maximo < 0 {
		return ErrCantidadInvalida
	}
	if maximo != 0 && maximo < minimo {
		return fmt.Errorf("%w: el máximo (%d) no puede ser menor que el mínimo (%d)", ErrCantidadInvalida, maximo, minimo)
	}
	if err := s.repo.FijarNivel(ctx, empresaID, articuloID, sedeID, minimo, maximo); err != nil {
		return err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_nivel", EntidadID: &articuloID,
		Accion: "FIJAR_NIVEL_INV", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{"sede_id": sedeID, "minimo": minimo, "maximo": maximo},
	})
	return nil
}

// NivelesDeArticulo devuelve los mínimos y máximos por sede de un artículo.
func (s *Service) NivelesDeArticulo(ctx context.Context, empresaID, articuloID string) ([]NivelSede, error) {
	return s.repo.NivelesDeArticulo(ctx, empresaID, articuloID)
}

// ── Existencias ─────────────────────────────────────────────────────────────

// Existencias arma la pantalla principal: qué hay, dónde, cuánto vale y qué hay que reponer.
func (s *Service) Existencias(ctx context.Context, empresaID string, f FiltroExistencias) (ResumenExistencias, error) {
	if f.ModoControl != "" && !modoValido(f.ModoControl) {
		return ResumenExistencias{}, ErrModoInvalido
	}
	filas, err := s.repo.Existencias(ctx, empresaID, f)
	if err != nil {
		return ResumenExistencias{}, err
	}

	// Filas arranca como slice vacío y no nil: un nil de Go llega al navegador como `null` y
	// `null.map()` rompe la pantalla completa. Pasó ya dos veces en este sistema.
	out := ResumenExistencias{Filas: []ExistenciaSede{}}
	valor, consignado := decimal.Zero, decimal.Zero
	sedes := map[string]bool{}

	for _, fila := range filas {
		fila.Estado = estadoDeNivel(fila.Cantidad, fila.Minimo, fila.Maximo)
		if f.SoloBajoMinimo && fila.Estado != NivelBajo && fila.Estado != NivelCerca {
			continue
		}
		v, err := decimal.NewFromString(fila.ValorCRC)
		if err != nil {
			return ResumenExistencias{}, fmt.Errorf("inventario: valor de %s: %w", fila.Articulo, err)
		}
		valor = valor.Add(v)
		if fila.ModoControl == ModoUnidad {
			out.UnidadesTotales += fila.Cantidad
		} else {
			out.CantidadesTotales += fila.Cantidad
		}
		if fila.Estado == NivelBajo {
			out.BajoMinimo++
		}
		// Lo consignado ya viene valorizado por el SQL con el costo real de cada unidad. Antes se
		// estimaba acá multiplicando el promedio de la fila por la cantidad de consignadas, y con
		// costos distintos dentro del mismo artículo el número salía mal.
		if fila.ValorConsignadoCRC != "" {
			vc, err := decimal.NewFromString(fila.ValorConsignadoCRC)
			if err != nil {
				return ResumenExistencias{}, fmt.Errorf("inventario: valor consignado de %s: %w", fila.Articulo, err)
			}
			consignado = consignado.Add(vc)
		}
		out.UnidadesConsignadas += fila.Consignadas
		if fila.SedeID != "" {
			sedes[fila.SedeID] = true
		}
		out.Filas = append(out.Filas, fila)
	}

	out.ValorTotalCRC = valor.StringFixed(2)
	out.ConsignadasCRC = consignado.StringFixed(2)
	out.ValorPropioCRC = valor.Sub(consignado).StringFixed(2)
	out.SedesConStock = len(sedes)
	out.Aviso, err = s.avisoExistencias(ctx, empresaID, out)
	if err != nil {
		return ResumenExistencias{}, err
	}
	return out, nil
}

// estadoDeNivel es el semáforo. Sin mínimo definido NO hay semáforo: decir «en rango» sobre un
// mínimo que nadie fijó es afirmar algo que no se sabe.
func estadoDeNivel(hay, minimo, maximo int) string {
	if minimo == 0 && maximo == 0 {
		return NivelSinNivel
	}
	switch {
	case minimo > 0 && hay < minimo:
		return NivelBajo
	case minimo > 0 && float64(hay) < float64(minimo)*umbralCercaDelMinimo:
		return NivelCerca
	case maximo > 0 && hay > maximo:
		return NivelSobre
	default:
		return NivelEnRango
	}
}

// avisoExistencias explica en una frase por qué el número puede no ser confiable.
func (s *Service) avisoExistencias(ctx context.Context, empresaID string, r ResumenExistencias) (string, error) {
	var partes []string

	sedes, err := s.repo.ContarSedes(ctx, empresaID)
	if err != nil {
		return "", err
	}
	if sedes == 0 {
		partes = append(partes, "todavía no hay sedes cargadas, y el inventario se organiza por sede: cargalas en el catálogo antes de registrar existencias")
	}

	// El aviso más importante de este módulo. Si nadie registra los servicios prestados, las
	// existencias solo suben: la pantalla se ve sana mientras en la bodega falta la mitad.
	hoy := ahoraCR().Format("2006-01-02")
	hace30 := ahoraCR().AddDate(0, 0, -30).Format("2006-01-02")
	servicios, err := s.repo.ContarServicios(ctx, empresaID, hace30, hoy)
	if err != nil {
		return "", err
	}
	if servicios == 0 && (r.UnidadesTotales > 0 || r.CantidadesTotales > 0) {
		partes = append(partes, "en los últimos 30 días no se registró ningún servicio prestado: si los funerales no se anotan, las existencias solo suben y este número queda por encima de lo que hay en bodega")
	}

	if r.BajoMinimo > 0 {
		partes = append(partes, fmt.Sprintf("%d artículo(s) están por debajo de su mínimo", r.BajoMinimo))
	}
	if len(partes) == 0 {
		return "", nil
	}
	return strings.Join(partes, " · ") + ".", nil
}

// Unidades lista las fichas físicas.
func (s *Service) Unidades(ctx context.Context, empresaID string, f FiltroUnidades) ([]Unidad, error) {
	if f.Estado != "" && !estadoUnidadValido(f.Estado) {
		return nil, fmt.Errorf("%w: %q", ErrEstadoUnidadInvalido, f.Estado)
	}
	if f.Limite <= 0 || f.Limite > 500 {
		f.Limite = 200
	}
	us, err := s.repo.ListarUnidades(ctx, empresaID, f)
	if err != nil {
		return nil, err
	}
	for i := range us {
		us[i].EstadoLegible = EtiquetaEstado(us[i].Estado)
	}
	return us, nil
}

func estadoUnidadValido(e string) bool {
	switch e {
	case EstadoDisponible, EstadoReservada, EstadoEnTransito, EstadoExhibicion,
		EstadoUsada, EstadoDanada, EstadoDevuelta, EstadoNoAparecio:
		return true
	}
	return false
}

// Movimientos devuelve el libro del inventario.
func (s *Service) Movimientos(ctx context.Context, empresaID string, f FiltroMovimientos) ([]Movimiento, error) {
	if f.Limite <= 0 || f.Limite > 1000 {
		f.Limite = 300
	}
	ms, err := s.repo.ListarMovimientos(ctx, empresaID, f)
	if err != nil {
		return nil, err
	}
	for i := range ms {
		ms[i].TipoLegible = EtiquetaTipo(ms[i].Tipo)
		if sumaAlStock(ms[i].Tipo) {
			ms[i].Signo = 1
		} else {
			ms[i].Signo = -1
		}
	}
	return ms, nil
}

// ── Entradas ────────────────────────────────────────────────────────────────

// RegistrarEntrada da de alta mercadería que llegó.
func (s *Service) RegistrarEntrada(ctx context.Context, empresaID string, e EntradaNueva, usuarioID string) (EntradaHecha, error) {
	if e.Cantidad <= 0 {
		return EntradaHecha{}, ErrCantidadInvalida
	}
	if e.SedeID == "" {
		return EntradaHecha{}, ErrSedeRequerida
	}
	if !reFecha.MatchString(e.Fecha) {
		return EntradaHecha{}, ErrFechaInvalida
	}
	costo, err := decimal.NewFromString(e.CostoUnitario)
	if err != nil {
		return EntradaHecha{}, fmt.Errorf("%w: %q", ErrCostoNegativo, e.CostoUnitario)
	}
	if costo.IsNegative() {
		return EntradaHecha{}, ErrCostoNegativo
	}
	e.CostoUnitario = costo.StringFixed(2)

	// Consignación: dos guardarraíles, y los dos existen porque el dato se estaba perdiendo.
	//
	// 1. Sin proveedor no hay a quién pagarle. La pantalla nunca mandaba el proveedor, así que
	//    `inv_unidad.proveedor_id` quedaba SIEMPRE en NULL: una consignada sin dueño es una cuenta
	//    por pagar sin destinatario, y el problema recién se descubre el día que hay que pagar.
	// 2. La consignación se lleva por unidad. Un artículo por cantidad no crea fichas, así que el
	//    `es_consignada` de una entrada por cantidad no se guardaba en ninguna parte y se perdía en
	//    silencio: peor que rechazarlo, porque el usuario cree que quedó marcado.
	if e.EsConsignada {
		if e.ProveedorID == "" {
			return EntradaHecha{}, ErrProveedorConsignadaRequerido
		}
		art, err := s.repo.ArticuloPorID(ctx, empresaID, e.ArticuloID)
		if err != nil {
			return EntradaHecha{}, err
		}
		if art.ModoControl != ModoUnidad {
			return EntradaHecha{}, &ConsignacionSoloPorUnidadError{Articulo: art.Nombre}
		}
	}

	hecha, err := s.repo.RegistrarEntrada(ctx, empresaID, e, usuarioID)
	if err != nil {
		return EntradaHecha{}, err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_movimiento", EntidadID: &hecha.MovimientoID,
		Accion: "ENTRADA_INVENTARIO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"articulo_id": e.ArticuloID, "sede_id": e.SedeID, "cantidad": e.Cantidad,
			"costo_unitario": e.CostoUnitario, "consignada": e.EsConsignada,
			"unidades": hecha.NumerosCreados,
		},
	})
	return hecha, nil
}

// ── El servicio prestado: lo que descarga el inventario ─────────────────────

// RegistrarServicio anota un funeral prestado y descarga lo que consumió.
func (s *Service) RegistrarServicio(ctx context.Context, empresaID string, sv ServicioNuevo, usuarioID string) (Servicio, error) {
	if sv.SedeID == "" {
		return Servicio{}, ErrSedeRequerida
	}
	if !reFecha.MatchString(sv.Fecha) {
		return Servicio{}, ErrFechaInvalida
	}
	if len(sv.Consumos) == 0 {
		return Servicio{}, ErrServicioSinConsumos
	}
	for _, c := range sv.Consumos {
		if c.ArticuloID == "" {
			return Servicio{}, ErrArticuloNoEncontrado
		}
		// Para los de unidad el número identifica el objeto; para los de cantidad hace falta cuántos.
		if c.UnidadNumero == "" && c.Cantidad <= 0 {
			return Servicio{}, ErrCantidadInvalida
		}
	}

	hecho, err := s.repo.RegistrarServicio(ctx, empresaID, sv, usuarioID)
	if err != nil {
		return Servicio{}, err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_servicio", EntidadID: &hecho.ID,
		Accion: "REGISTRAR_SERVICIO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"numero": hecho.Numero, "sede_id": sv.SedeID, "fecha": sv.Fecha,
			"lineas": len(hecho.Consumos), "costo_producto": hecho.CostoProductoCRC,
		},
	})
	return hecho, nil
}

// Servicios lista los servicios del rango.
func (s *Service) Servicios(ctx context.Context, empresaID, desde, hasta string) ([]Servicio, error) {
	if desde != "" && !reFecha.MatchString(desde) {
		return nil, ErrFechaInvalida
	}
	if hasta != "" && !reFecha.MatchString(hasta) {
		return nil, ErrFechaInvalida
	}
	return s.repo.ListarServicios(ctx, empresaID, desde, hasta)
}

// ── Ajustes y bajas ─────────────────────────────────────────────────────────

// RegistrarAjuste corrige una existencia o cambia el estado de una unidad. Exige motivo: un ajuste
// sin explicación es justo el movimiento que después nadie puede auditar.
func (s *Service) RegistrarAjuste(ctx context.Context, empresaID string, a AjusteNuevo, usuarioID string) error {
	if strings.TrimSpace(a.Motivo) == "" {
		return ErrMotivoRequerido
	}
	if !reFecha.MatchString(a.Fecha) {
		return ErrFechaInvalida
	}
	// O se ajusta una cantidad, o se cambia el estado de una unidad; no las dos cosas.
	if a.UnidadNumero == "" {
		if a.Diferencia == 0 {
			return fmt.Errorf("%w: un ajuste de cero no cambia nada", ErrCantidadInvalida)
		}
		if a.SedeID == "" {
			return ErrSedeRequerida
		}
	} else if a.NuevoEstado != "" && !estadoUnidadValido(a.NuevoEstado) {
		return fmt.Errorf("inventario: «%s» no es un estado de unidad", a.NuevoEstado)
	}

	if err := s.repo.RegistrarAjuste(ctx, empresaID, a, usuarioID); err != nil {
		return err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_movimiento", EntidadID: &a.ArticuloID,
		Accion: "AJUSTE_INVENTARIO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"sede_id": a.SedeID, "unidad": a.UnidadNumero, "diferencia": a.Diferencia,
			"nuevo_estado": a.NuevoEstado, "motivo": a.Motivo,
		},
	})
	return nil
}

// ── Traslados ───────────────────────────────────────────────────────────────

// CrearTraslado envía mercadería a otra sede. Lo enviado queda EN TRÁNSITO: no está en ninguna de
// las dos sedes hasta que alguien lo recibe, y así una unidad perdida en el camino tiene dueño.
func (s *Service) CrearTraslado(ctx context.Context, empresaID string, t TrasladoNuevo, usuarioID string) (Traslado, error) {
	if t.SedeOrigenID == "" || t.SedeDestinoID == "" {
		return Traslado{}, ErrSedeRequerida
	}
	if t.SedeOrigenID == t.SedeDestinoID {
		return Traslado{}, ErrMismaSede
	}
	if !reFecha.MatchString(t.Fecha) {
		return Traslado{}, ErrFechaInvalida
	}
	if len(t.Lineas) == 0 {
		return Traslado{}, fmt.Errorf("inventario: el traslado tiene que llevar al menos una línea")
	}
	for _, l := range t.Lineas {
		if l.UnidadNumero == "" && l.Cantidad <= 0 {
			return Traslado{}, ErrCantidadInvalida
		}
	}

	hecho, err := s.repo.CrearTraslado(ctx, empresaID, t, usuarioID)
	if err != nil {
		return Traslado{}, err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_traslado", EntidadID: &hecho.ID,
		Accion: "ENVIAR_TRASLADO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"numero": hecho.Numero, "origen": t.SedeOrigenID, "destino": t.SedeDestinoID,
			"lineas": len(t.Lineas),
		},
	})
	return hecho, nil
}

// RecibirTraslado confirma la llegada. Recién acá lo trasladado vuelve a existir en una sede.
func (s *Service) RecibirTraslado(ctx context.Context, empresaID, id, fecha, usuarioID string) error {
	if fecha == "" {
		fecha = ahoraCR().Format("2006-01-02")
	}
	if !reFecha.MatchString(fecha) {
		return ErrFechaInvalida
	}
	if err := s.repo.RecibirTraslado(ctx, empresaID, id, fecha, usuarioID); err != nil {
		return err
	}
	s.registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "inv_traslado", EntidadID: &id,
		Accion: "RECIBIR_TRASLADO", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"recibido_en": fecha},
	})
	return nil
}

// Traslados lista los traslados, opcionalmente por estado.
func (s *Service) Traslados(ctx context.Context, empresaID, estado string) ([]Traslado, error) {
	if estado != "" && estado != TrasladoEnTransito && estado != TrasladoRecibido && estado != TrasladoCancelado {
		return nil, fmt.Errorf("inventario: «%s» no es un estado de traslado", estado)
	}
	return s.repo.ListarTraslados(ctx, empresaID, estado)
}

// ── Reposición y rotación ───────────────────────────────────────────────────

// Reposicion dice qué hay que pedir. Con poca historia NO inventa una cantidad: devuelve el faltante
// hasta el mínimo y lo dice, porque una sugerencia sin datos es una orden que nadie puede discutir.
func (s *Service) Reposicion(ctx context.Context, empresaID, sedeID string, semanas int) ([]SugerenciaPedido, error) {
	if semanas <= 0 || semanas > 52 {
		semanas = 8
	}
	hoy := ahoraCR()
	desde := hoy.AddDate(0, 0, -7*semanas).Format("2006-01-02")
	servicios, err := s.repo.ContarServicios(ctx, empresaID, desde, hoy.Format("2006-01-02"))
	if err != nil {
		return nil, err
	}
	filas, err := s.repo.Reposicion(ctx, empresaID, sedeID, semanas)
	if err != nil {
		return nil, err
	}
	hayHistoria := servicios >= serviciosMinimosParaSugerir
	for i := range filas {
		f := &filas[i]
		if hayHistoria {
			f.Sugerido = sugerirConConsumo(*f)
			f.DeDondeSale = explicarSugerido(*f, semanas)
		} else {
			// Sin historia no se usa el consumo: se pide lo que falta para el mínimo y se dice por qué.
			f.ConsumoSemanal = ""
			f.Sugerido = maxInt(0, f.Minimo-f.Hay)
			f.DeDondeSale = fmt.Sprintf(
				"solo hay %d servicio(s) registrados en %d semanas: todavía no se puede calcular el consumo, así que esto es lo que falta para llegar al mínimo",
				servicios, semanas)
		}
		// El costo se calcula DESPUÉS de fijar la cantidad, y sobre esa misma cantidad. Cuando el
		// repositorio lo calculaba por su cuenta, una fila mostraba «1 unidad · ₡576.400» porque el
		// servicio le había cambiado la cantidad sin recalcular el costo.
		cu, err := decimal.NewFromString(f.CostoUnitarioCRC)
		if err != nil {
			cu = decimal.Zero
		}
		f.CostoEstimadoCRC = cu.Mul(decimal.NewFromInt(int64(f.Sugerido))).StringFixed(2)
	}
	return filas, nil
}

// sugerirConConsumo calcula cuánto pedir cuando SÍ hay historia: cubrir el consumo del tiempo de
// entrega, sin bajar del mínimo ni pasar del máximo.
func sugerirConConsumo(f SugerenciaPedido) int {
	objetivo := f.Minimo
	if f.Maximo > 0 {
		objetivo = f.Maximo
	}
	sug := objetivo - f.Hay
	cs, err := decimal.NewFromString(f.ConsumoSemanal)
	if err == nil && cs.IsPositive() {
		porEntrega := int(cs.Mul(decimal.NewFromInt(semanasDeEntrega)).Ceil().IntPart())
		if porEntrega > sug {
			sug = porEntrega
		}
		if f.Maximo > 0 && f.Hay+sug > f.Maximo {
			sug = f.Maximo - f.Hay
		}
	}
	return maxInt(0, sug)
}

// explicarSugerido pone en palabras de dónde salió la cantidad. El número sin la frase obliga a
// confiar a ciegas.
func explicarSugerido(f SugerenciaPedido, semanas int) string {
	cs, err := decimal.NewFromString(f.ConsumoSemanal)
	if err != nil || cs.IsZero() {
		return fmt.Sprintf("no tuvo salidas en %d semanas: lo sugerido es lo que falta para el mínimo", semanas)
	}
	tope := ""
	if f.Maximo > 0 {
		tope = fmt.Sprintf(", sin pasar del máximo de %d", f.Maximo)
	}
	return fmt.Sprintf("consumo de %s por semana × %d semanas de entrega, con %d en existencia y mínimo de %d%s",
		cs.StringFixed(1), semanasDeEntrega, f.Hay, f.Minimo, tope)
}

// Rotacion mide qué se mueve y qué está detenido.
func (s *Service) Rotacion(ctx context.Context, empresaID, desde, hasta string) ([]RotacionArticulo, error) {
	if !reFecha.MatchString(desde) || !reFecha.MatchString(hasta) {
		return nil, ErrFechaInvalida
	}
	if desde > hasta {
		return nil, fmt.Errorf("%w: el desde no puede ser posterior al hasta", ErrFechaInvalida)
	}
	return s.repo.Rotacion(ctx, empresaID, desde, hasta)
}

func maxInt(a, b int) int {
	if a > b {
		return a
	}
	return b
}
