package conta

import (
	"context"
	"sort"
	"strings"

	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/shared"
)

// Service aplica las reglas del plan de cuentas.
type Service struct {
	repo  Repository
	audit *shared.Audit
	log   *zap.Logger
}

// NewService construye el servicio.
func NewService(repo Repository, audit *shared.Audit, log *zap.Logger) *Service {
	return &Service{repo: repo, audit: audit, log: log}
}

// Cuentas devuelve el plan en lista plana, ordenado por código.
func (s *Service) Cuentas(ctx context.Context, empresaID string, soloActivas bool) ([]Cuenta, error) {
	return s.repo.Cuentas(ctx, empresaID, soloActivas)
}

// Arbol devuelve el plan como árbol, para la pantalla.
//
// Se arma acá y no en SQL a propósito: una consulta recursiva devuelve filas que el cliente tiene
// que volver a anidar igual, y la jerarquía del plan de cuentas es la pieza que más se mira. Con
// 258 cuentas el costo es irrelevante y el resultado llega listo para dibujar.
func (s *Service) Arbol(ctx context.Context, empresaID string, soloActivas bool) ([]*Cuenta, error) {
	planas, err := s.repo.Cuentas(ctx, empresaID, soloActivas)
	if err != nil {
		return nil, err
	}
	return ArbolDe(planas), nil
}

// ArbolDe anida una lista plana por `PadreID`.
//
// Una cuenta cuya madre no está en la lista se devuelve como raíz en vez de desaparecer: pasa al
// filtrar por activas —una madre apagada con hijas vivas— y perder las hijas en silencio sería
// mostrar un plan incompleto sin decirlo.
func ArbolDe(planas []Cuenta) []*Cuenta {
	porID := make(map[string]*Cuenta, len(planas))
	for i := range planas {
		c := planas[i]
		c.Hijas = nil
		porID[c.ID] = &c
	}
	raices := make([]*Cuenta, 0)
	for _, c := range porID {
		if c.PadreID == nil {
			raices = append(raices, c)
			continue
		}
		madre, ok := porID[*c.PadreID]
		if !ok {
			raices = append(raices, c)
			continue
		}
		madre.Hijas = append(madre.Hijas, c)
	}
	ordenarPorCodigo(raices)
	return raices
}

func ordenarPorCodigo(cs []*Cuenta) {
	sort.Slice(cs, func(i, j int) bool { return cs[i].Codigo < cs[j].Codigo })
	for _, c := range cs {
		ordenarPorCodigo(c.Hijas)
	}
}

// Validar normaliza y comprueba lo que no depende de la base.
//
// Lo que SÍ depende de la base —que la madre exista, que no acepte movimiento, que el código no
// esté repetido— lo verifica el repositorio y, en última instancia, el disparador de la migración
// 0109. Acá se atrapa lo que se puede atrapar antes de viajar.
func (c *Cuenta) Validar() error {
	c.Codigo = strings.TrimSpace(c.Codigo)
	c.Nombre = strings.TrimSpace(c.Nombre)
	c.CodigoSoftland = strings.TrimSpace(c.CodigoSoftland)
	if c.Codigo == "" {
		return ErrCodigoRequerido
	}
	if c.Nombre == "" {
		return ErrNombreRequerido
	}
	clase, ok := claseDelCodigo(c.Codigo)
	if !ok {
		return ErrClaseDesconocida
	}
	// La clase y el nivel SIEMPRE se derivan del código, aunque vengan en el pedido: son función
	// del código y aceptar otra cosa permitiría guardar una cuenta «1-…» marcada como pasivo.
	c.Clase = clase
	c.Nivel = nivelDelCodigo(c.Codigo)
	if c.Naturaleza != Deudora && c.Naturaleza != Acreedora {
		c.Naturaleza = naturalezaDeLaClase(clase)
	}
	return nil
}

// CrearCuenta agrega una cuenta al plan.
func (s *Service) CrearCuenta(ctx context.Context, empresaID string, c Cuenta, usuarioID string) (Cuenta, error) {
	if err := c.Validar(); err != nil {
		return Cuenta{}, err
	}
	// La empresa sale del token, nunca del cuerpo: una cuenta creada desde una empresa pertenece
	// a esa empresa. Las del grupo se crean al importar, que es una acción distinta y explícita.
	emp := empresaID
	c.EmpresaID = &emp
	creada, err := s.repo.CrearCuenta(ctx, c, usuarioID)
	if err != nil {
		return Cuenta{}, err
	}
	s.auditar(ctx, empresaID, creada.ID, "CREAR_CUENTA", usuarioID, map[string]any{
		"codigo": creada.Codigo, "nombre": creada.Nombre, "clase": creada.Clase,
	})
	return creada, nil
}

// ActualizarCuenta cambia el nombre, la naturaleza, si acepta movimiento o si está activa.
//
// El CÓDIGO no se cambia acá. Cambiarlo es mover la cuenta de lugar en el árbol, y eso arrastra a
// sus hijas y a todo lo contabilizado en ella: es otra operación, con otras consecuencias, y no
// debe esconderse detrás de «editar».
func (s *Service) ActualizarCuenta(ctx context.Context, empresaID string, c Cuenta, usuarioID string) (Cuenta, error) {
	c.Nombre = strings.TrimSpace(c.Nombre)
	if c.Nombre == "" {
		return Cuenta{}, ErrNombreRequerido
	}
	if c.Naturaleza != Deudora && c.Naturaleza != Acreedora {
		return Cuenta{}, ErrClaseDesconocida
	}
	act, err := s.repo.ActualizarCuenta(ctx, empresaID, c)
	if err != nil {
		return Cuenta{}, err
	}
	s.auditar(ctx, empresaID, act.ID, "ACTUALIZAR_CUENTA", usuarioID, map[string]any{
		"codigo": act.Codigo, "nombre": act.Nombre,
		"acepta_movimiento": act.AceptaMovimiento, "activo": act.Activo,
	})
	return act, nil
}

// BorrarCuenta la elimina.
//
// Borrado físico, y hoy es seguro: en la Fase 1 nada cuelga de una cuenta. Cuando existan
// asientos, el repositorio devolverá ErrCuentaConMovimientos y el camino será apagarla.
func (s *Service) BorrarCuenta(ctx context.Context, empresaID, id, usuarioID string) error {
	if err := s.repo.BorrarCuenta(ctx, empresaID, id); err != nil {
		return err
	}
	s.auditar(ctx, empresaID, id, "BORRAR_CUENTA", usuarioID, nil)
	return nil
}

func (s *Service) auditar(ctx context.Context, empresaID, id, accion, usuarioID string, detalle map[string]any) {
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "cuenta_contable", EntidadID: &id,
		Accion: accion, UsuarioID: &usuarioID, ValorNuevo: detalle,
	})
}
