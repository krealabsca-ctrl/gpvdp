package nomina

// Notificaciones de RRHH: la boleta de pago y el aviso de vacaciones.
//
// El texto lo define la plantilla de la empresa (Configuración → Notificaciones); acá se llenan
// los datos y se envía. Antes de esto, Nómina no notificaba nada: la boleta se leía en pantalla.
//
// Dos guardarraíles, porque acá viajan salarios:
//   · Solo se envía al correo registrado en la ficha del empleado. Sin correo, no se envía y se
//     reporta — nunca a una dirección "parecida".
//   · Cada envío queda en la auditoría con el empleado y el período.

import (
	"context"
	"errors"
	"fmt"
	"strings"

	"github.com/gpvdp/erp/internal/plantillas"
	"github.com/gpvdp/erp/internal/shared"
	"go.uber.org/zap"
)

var (
	// ErrCorreoNoConfigurado: el servidor de correo no está conectado; no se puede notificar.
	ErrCorreoNoConfigurado = errors.New("nomina: el envío de correos no está configurado")
	// ErrEmpleadoSinCorreo: la ficha del empleado no tiene correo. No se adivina una dirección.
	ErrEmpleadoSinCorreo = errors.New("nomina: el colaborador no tiene correo registrado en su ficha")
)

// VacacionAviso son los datos del disfrute para el correo. El saldo lo completa el servicio con
// el cálculo derivado que ya existe (no se recalcula acá).
type VacacionAviso struct {
	EmpleadoID    string
	Nombre        string
	Email         string
	Dias          string
	FechaInicio   string
	FechaFin      string
	SaldoDias     string
	Observaciones string
}

// mesesES nombra el período en el correo (el colaborador lee «Julio 2026», no «2026-07»).
var mesesES = [...]string{"", "Enero", "Febrero", "Marzo", "Abril", "Mayo", "Junio",
	"Julio", "Agosto", "Septiembre", "Octubre", "Noviembre", "Diciembre"}

// periodoTexto escribe el período como «Julio 2026».
func periodoTexto(anio, mes int) string {
	if mes < 1 || mes > 12 {
		return fmt.Sprintf("%04d-%02d", anio, mes)
	}
	return fmt.Sprintf("%s %d", mesesES[mes], anio)
}

// Plantillero arma asunto y cuerpo desde la plantilla vigente de la empresa.
type Plantillero interface {
	Armar(ctx context.Context, empresaID, clave string, valores map[string]string) (string, string, error)
}

// Correo envía un mensaje de texto.
type Correo interface {
	Enviar(to, asunto, cuerpo string) error
}

// SetNotificaciones conecta el armado de texto y el envío. Sin esto, los endpoints de envío
// responden que no está configurado en vez de fallar de forma rara.
func (s *Service) SetNotificaciones(p Plantillero, c Correo) {
	s.plantillas = p
	s.correo = c
}

// ResultadoEnvio es el reporte de un envío masivo de boletas.
type ResultadoEnvio struct {
	Enviados int `json:"enviados"`
	// SinCorreo: empleados que no tienen correo en su ficha (no se les pudo avisar).
	SinCorreo []string `json:"sin_correo"`
	// Fallidos: el correo existe pero el envío falló (servidor caído, dirección rechazada).
	Fallidos []string `json:"fallidos"`
}

// EnviarBoletas manda la boleta de pago a cada empleado de la corrida que tenga correo.
//
// No se detiene ante un fallo individual: manda las que puede y reporta el resto. Un servidor
// SMTP caído no debe dejar a 12 personas sin su boleta porque la primera falló.
func (s *Service) EnviarBoletas(ctx context.Context, empresaID, corridaID, usuarioID string) (ResultadoEnvio, error) {
	if s.correo == nil {
		return ResultadoEnvio{}, ErrCorreoNoConfigurado
	}
	corrida, err := s.repo.CorridaPorID(ctx, empresaID, corridaID)
	if err != nil {
		return ResultadoEnvio{}, err
	}
	lineas, err := s.repo.LineasCorrida(ctx, empresaID, corridaID)
	if err != nil {
		return ResultadoEnvio{}, err
	}
	correos, err := s.repo.CorreosEmpleados(ctx, empresaID)
	if err != nil {
		return ResultadoEnvio{}, err
	}

	res := ResultadoEnvio{SinCorreo: []string{}, Fallidos: []string{}}
	for _, l := range lineas {
		para := strings.TrimSpace(correos[l.EmpleadoID])
		if para == "" {
			res.SinCorreo = append(res.SinCorreo, l.Nombre)
			continue
		}
		asunto, cuerpo, err := s.textoBoleta(ctx, empresaID, corrida, l)
		if err != nil {
			return ResultadoEnvio{}, err
		}
		if err := s.correo.Enviar(para, asunto, cuerpo); err != nil {
			s.log.Warn("no se pudo enviar la boleta",
				zap.String("empleado", l.Nombre), zap.Error(err))
			res.Fallidos = append(res.Fallidos, l.Nombre)
			continue
		}
		res.Enviados++
	}

	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "corrida_nomina", EntidadID: &corridaID,
		Accion: "ENVIAR_BOLETAS", UsuarioID: &usuarioID,
		ValorNuevo: map[string]any{
			"periodo": periodoTexto(corrida.Anio, corrida.Mes), "enviados": res.Enviados,
			"sin_correo": len(res.SinCorreo), "fallidos": len(res.Fallidos),
		},
	})
	return res, nil
}

// textoBoleta arma el correo de la boleta de un empleado.
func (s *Service) textoBoleta(ctx context.Context, empresaID string, c Corrida, l LineaCorrida) (string, string, error) {
	valores := map[string]string{
		"NOMBRE_EMPLEADO":   l.Nombre,
		"IDENTIFICACION":    l.Identificacion,
		"PUESTO":            l.Puesto,
		"PERIODO":           periodoTexto(c.Anio, c.Mes),
		"FECHA_PAGO":        fechaLegible(c.FechaPago),
		"SALARIO_BRUTO":     colones(l.Bruto),
		"CCSS_OBRERO":       colones(l.CCSSObrero),
		"RENTA":             colones(l.Renta),
		"OTRAS_DEDUCCIONES": colones(l.Deducciones),
		"ADELANTO":          colones(l.Adelanto),
		"NETO":              colones(l.Neto),
	}
	return s.armar(ctx, empresaID, plantillas.ClaveNominaBoleta, valores)
}

// EnviarAvisoVacaciones manda el aviso de un disfrute de vacaciones al colaborador.
func (s *Service) EnviarAvisoVacaciones(ctx context.Context, empresaID, vacacionID, usuarioID string) error {
	if s.correo == nil {
		return ErrCorreoNoConfigurado
	}
	av, err := s.repo.VacacionParaAviso(ctx, empresaID, vacacionID)
	if err != nil {
		return err
	}
	if strings.TrimSpace(av.Email) == "" {
		return ErrEmpleadoSinCorreo
	}
	// El saldo sale del cálculo derivado que ya usa la pantalla de vacaciones, con los días por
	// mes de los parámetros del año del disfrute.
	if dias, err := s.diasVacacionesPorMes(ctx, empresaID, anioDeFecha(av.FechaInicio)); err == nil {
		if saldo, err := s.repo.SaldoVacacionesEmpleado(ctx, empresaID, av.EmpleadoID, dias); err == nil {
			av.SaldoDias = saldo.Pendiente
		}
	}
	valores := map[string]string{
		"NOMBRE_EMPLEADO": av.Nombre,
		"DIAS":            av.Dias,
		"FECHA_INICIO":    fechaLegible(av.FechaInicio),
		"FECHA_FIN":       fechaLegible(av.FechaFin),
		"SALDO_DIAS":      av.SaldoDias,
		"OBSERVACIONES":   av.Observaciones,
	}
	asunto, cuerpo, err := s.armar(ctx, empresaID, plantillas.ClaveNominaVacaciones, valores)
	if err != nil {
		return err
	}
	if err := s.correo.Enviar(av.Email, asunto, cuerpo); err != nil {
		return fmt.Errorf("nomina: enviar aviso de vacaciones: %w", err)
	}
	s.audit.Registrar(ctx, shared.Evento{
		EmpresaID: &empresaID, Entidad: "vacacion", EntidadID: &vacacionID,
		Accion: "ENVIAR_AVISO_VACACIONES", UsuarioID: &usuarioID,
		ValorNuevo: map[string]string{"empleado": av.Nombre, "dias": av.Dias},
	})
	return nil
}

// armar usa la plantilla de la empresa; sin el servicio conectado, el texto de fábrica.
func (s *Service) armar(ctx context.Context, empresaID, clave string, valores map[string]string) (string, string, error) {
	if s.plantillas != nil {
		return s.plantillas.Armar(ctx, empresaID, clave, valores)
	}
	t, ok := plantillas.TipoPorClave(clave)
	if !ok {
		return "", "", plantillas.ErrTipoDesconocido
	}
	return plantillas.Render(t.AsuntoDefault, valores), plantillas.Render(t.CuerpoDefault, valores), nil
}

// colones formatea un monto para el texto del correo (₡1 234 567,89).
func colones(monto string) string {
	return "₡" + miles(monto)
}

// miles agrupa la parte entera con espacio fino y usa coma decimal, como se lee en Costa Rica.
func miles(monto string) string {
	entero, decimales := monto, "00"
	if i := strings.IndexByte(monto, '.'); i >= 0 {
		entero, decimales = monto[:i], monto[i+1:]
	}
	negativo := strings.HasPrefix(entero, "-")
	entero = strings.TrimPrefix(entero, "-")
	var b strings.Builder
	for i, r := range entero {
		if i > 0 && (len(entero)-i)%3 == 0 {
			b.WriteRune(' ')
		}
		b.WriteRune(r)
	}
	out := b.String() + "," + decimales
	if negativo {
		return "-" + out
	}
	return out
}

// anioDeFecha saca el año de una fecha YYYY-MM-DD (0 si no lo tiene, y el servicio cae en el
// parámetro por defecto).
func anioDeFecha(iso string) int {
	if len(iso) < 4 {
		return 0
	}
	anio := 0
	for _, r := range iso[:4] {
		if r < '0' || r > '9' {
			return 0
		}
		anio = anio*10 + int(r-'0')
	}
	return anio
}

// fechaLegible pasa 2026-07-31 a 31/07/2026. Si no tiene ese formato, se devuelve tal cual.
func fechaLegible(iso string) string {
	if len(iso) < 10 {
		return iso
	}
	return iso[8:10] + "/" + iso[5:7] + "/" + iso[:4]
}
