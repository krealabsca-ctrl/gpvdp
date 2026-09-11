// Comando api: servidor HTTP del ERP GPVDP.
// Aplica migraciones al arrancar y (opcional) siembra datos base.
package main

import (
	"context"
	"errors"
	"flag"
	"fmt"
	"net/http"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/shopspring/decimal"
	"go.uber.org/zap"

	"github.com/gpvdp/erp/internal/auth"
	"github.com/gpvdp/erp/internal/bancos"
	"github.com/gpvdp/erp/internal/config"
	"github.com/gpvdp/erp/internal/cxc"
	"github.com/gpvdp/erp/internal/cxp"
	"github.com/gpvdp/erp/internal/database"
	"github.com/gpvdp/erp/internal/grupo"
	"github.com/gpvdp/erp/internal/inventario"
	"github.com/gpvdp/erp/internal/logging"
	"github.com/gpvdp/erp/internal/nomina"
	"github.com/gpvdp/erp/internal/plantillas"
	"github.com/gpvdp/erp/internal/rbac"
	"github.com/gpvdp/erp/internal/seed"
	"github.com/gpvdp/erp/internal/server"
	"github.com/gpvdp/erp/internal/shared"
	"github.com/gpvdp/erp/migrations"
)

func main() {
	migrateOnly := flag.Bool("migrate-only", false, "aplica migraciones y termina")
	flag.Parse()
	if err := run(*migrateOnly); err != nil {
		fmt.Fprintln(os.Stderr, "error fatal:", err)
		os.Exit(1)
	}
}

func run(migrateOnly bool) error {
	cfg, err := config.Load()
	if err != nil {
		return err
	}
	logger, err := logging.New(cfg.Env)
	if err != nil {
		return err
	}
	defer func() { _ = logger.Sync() }()

	// Contexto raíz cancelable por señal: apaga el scheduler BCCR (y cualquier
	// trabajo en curso) ANTES de cerrar el pool en el shutdown graceful.
	ctx, stop := signal.NotifyContext(context.Background(), syscall.SIGINT, syscall.SIGTERM)
	defer stop()

	pool, err := connectWithRetry(ctx, cfg.DatabaseURL, logger)
	if err != nil {
		return err
	}
	defer pool.Close()

	logger.Info("aplicando migraciones")
	if err := database.RunMigrations(migrations.FS, cfg.DatabaseURL); err != nil {
		return err
	}
	if migrateOnly {
		logger.Info("migraciones aplicadas (migrate-only); saliendo")
		return nil
	}

	if cfg.SeedOnStart {
		if err := seed.Run(ctx, pool, logger, cfg); err != nil {
			logger.Error("seed inicial falló (se continúa)", zap.Error(err))
		}
	}

	repo := auth.NewRepository(pool)
	svc := auth.NewService(repo, cfg.JWTSecret, cfg.AccessTTL, cfg.RefreshTTL)
	audit := shared.NewAudit(pool, logger)
	authH := auth.NewHandler(svc, audit, logger)

	// RBAC (matriz permiso × rol × empresa): siembra catálogo + matriz por defecto
	// (idempotente, en cada arranque) y provee el checker para el router.
	rbacSvc := rbac.NewService(rbac.NewRepository(pool), audit)
	if err := rbacSvc.EnsureDefaults(ctx); err != nil {
		logger.Error("rbac: no se pudieron sembrar los permisos por defecto (se continúa)", zap.Error(err))
	}
	rbacH := rbac.NewHandler(rbacSvc)

	bancosSvc := bancos.NewService(bancos.NewRepository(pool), audit, logger, cfg.CierreBloqueante)
	// Sync BCCR (Fase D): inyecta el fetcher solo si hay credenciales; sin ellas el
	// motor de TC sigue 100% manual. El scheduler corre los días 1/15/último (§22).
	if fetcher := bancos.NewBCCRClient(cfg.BCCRWSURL, cfg.BCCREmail, cfg.BCCRToken, cfg.BCCRIndicador, cfg.BCCRTimeout); fetcher != nil {
		bancosSvc.SetBCCR(fetcher)
		if cfg.BCCRSyncEnabled {
			go correrSchedulerBCCR(ctx, bancosSvc, logger)
			logger.Info("scheduler BCCR activo (días 1/15/último)")
		} else {
			logger.Info("BCCR configurado pero scheduler desactivado (BCCR_SYNC_ENABLED=false); sync manual disponible")
		}
	} else {
		logger.Info("BCCR sin credenciales; tipo de cambio 100% manual")
	}
	bancosH := bancos.NewHandler(bancosSvc, logger)
	cxpSvc := cxp.NewService(cxp.NewRepository(pool), audit, logger)
	cxpSvc.SetMailer(cxp.NewMailer(cfg.SMTPAddr, cfg.SMTPFrom, cfg.SMTPUser, cfg.SMTPPass, logger))
	cxpSvc.SetPermisos(rbacSvc) // scoping por área: sin cxp.ver_todo, el validador solo ve su departamento
	// Siembra el set base de departamentos por empresa (idempotente, cada arranque).
	if err := cxpSvc.EnsureDepartamentos(ctx); err != nil {
		logger.Error("cxp: no se pudieron sembrar los departamentos base (se continúa)", zap.Error(err))
	}
	cxpH := cxp.NewHandler(cxpSvc, logger)
	// Huella Bancos↔CxP: Bancos es dueño del movimiento y CxP de la huella. Con este puerto,
	// al importar el estado de cuenta los pagos hechos por la macro se emparejan solos con su
	// factura (adaptador porque cada módulo tiene su propio tipo de resultado).
	bancosSvc.SetConciliadorCxP(conciliadorCxP{cxpSvc})

	// Plantillas de las notificaciones: el TEXTO de los correos es configuración por empresa.
	// CxP y RRHH le piden a este servicio el asunto y el cuerpo antes de enviar.
	plantillasSvc := plantillas.NewService(plantillas.NewRepository(pool), audit)
	plantillasH := plantillas.NewHandler(plantillasSvc)
	cxpSvc.SetPlantillas(plantillasSvc)

	// RRHH / Nómina (Fase 3): siembra el catálogo base de conceptos (idempotente).
	// Los de sistema nacen con banderas bloqueadas: comisiones/bonos habituales SON salario.
	nominaSvc := nomina.NewService(nomina.NewRepository(pool), audit, logger)
	if err := nominaSvc.EnsureConceptos(ctx); err != nil {
		logger.Error("nomina: no se pudieron sembrar los conceptos base (se continúa)", zap.Error(err))
	}
	// RRHH notifica boletas y vacaciones por correo (antes no enviaba nada).
	nominaSvc.SetNotificaciones(plantillasSvc, shared.NewMailer(cfg.SMTPAddr, cfg.SMTPFrom, cfg.SMTPUser, cfg.SMTPPass, logger))
	// ── Cuentas por cobrar. El scoping por sede se resuelve con la misma matriz RBAC.
	cxcSvc := cxc.NewService(cxc.NewRepository(pool), audit, logger)
	cxcSvc.SetPermisos(rbacSvc)
	cxcH := cxc.NewHandler(cxcSvc, logger)
	inventarioSvc := inventario.NewService(inventario.NewRepository(pool), audit, logger)
	// Consignación: al usar un cofre del proveedor nace una cuenta por pagar. Inventario declara el
	// puerto y acá se satisface con CxP, igual que Bancos con la conciliación por huella. Si esta
	// línea no estuviera, el módulo seguiría funcionando y solo quedaría inhabilitado el botón de
	// facturar —la cola de «qué se le debe al proveedor» vale por sí sola—.
	inventarioSvc.SetFacturadorCxP(facturadorCxP{svc: cxpSvc})
	inventarioH := inventario.NewHandler(inventarioSvc, logger)

	nominaH := nomina.NewHandler(nominaSvc, logger)

	// ── Vista consolidada del grupo ──
	// Solo lee, y es la única que cruza empresas. No recibe ningún otro servicio: resuelve el alcance
	// desde las membresías del usuario, así que no otorga acceso nuevo ni depende de ningún módulo.
	grupoH := grupo.NewHandler(grupo.NewService(grupo.NewRepository(pool), logger), logger)

	router := server.NewRouter(cfg, logger, authH, bancosH, cxpH, cxcH, nominaH, rbacH, plantillasH, inventarioH, grupoH, rbacSvc)

	srv := &http.Server{
		Addr:    ":" + cfg.Port,
		Handler: router,
		// Timeouts finitos contra conexiones lentas/colgadas (Slowloris). Los valores dejan aire
		// a los caminos legítimamente largos: subir un .xlsx grande (ReadTimeout) y clasificar
		// decenas de miles de filas (WriteTimeout, por encima de los 300 s de Caddy).
		ReadHeaderTimeout: 10 * time.Second,
		ReadTimeout:       120 * time.Second,
		WriteTimeout:      310 * time.Second,
		IdleTimeout:       120 * time.Second,
	}

	go func() {
		logger.Info("servidor escuchando", zap.String("addr", srv.Addr))
		if err := srv.ListenAndServe(); err != nil && !errors.Is(err, http.ErrServerClosed) {
			logger.Fatal("error al escuchar", zap.Error(err))
		}
	}()

	<-ctx.Done()
	stop() // restaura el comportamiento por defecto de la señal (2.º Ctrl-C mata)

	logger.Info("apagando servidor…")
	shutdownCtx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
	defer cancel()
	if err := srv.Shutdown(shutdownCtx); err != nil {
		return fmt.Errorf("apagado: %w", err)
	}
	return nil
}

// correrSchedulerBCCR dispara el sync del BCCR una vez al día en días 1/15/último.
// Chequea cada 6h (barato). El día solo se marca como hecho si TODAS las empresas
// sincronizaron: ante fallo (BCCR caído, red), el siguiente tick REINTENTA — es
// idempotente porque el upsert BCCR jamás pisa un override MANUAL. Nunca tumba el servidor.
func correrSchedulerBCCR(ctx context.Context, svc *bancos.Service, logger *zap.Logger) {
	var ultimoDiaOK string
	tick := time.NewTicker(6 * time.Hour)
	defer tick.Stop()
	revisar := func() {
		ahora := time.Now()
		hoy := ahora.Format("2006-01-02")
		if hoy == ultimoDiaOK || !bancos.EsDiaDeSync(ahora) {
			return
		}
		logger.Info("sync BCCR programado", zap.String("fecha", hoy))
		if svc.SyncProgramado(ctx, hoy) {
			ultimoDiaOK = hoy
		} else {
			logger.Warn("sync BCCR incompleto; se reintentará en el próximo ciclo", zap.String("fecha", hoy))
		}
	}
	revisar() // al arrancar, por si hoy es día de captura
	for {
		select {
		case <-ctx.Done():
			return
		case <-tick.C:
			revisar()
		}
	}
}

// connectWithRetry espera a que PostgreSQL esté listo (útil con docker compose).
func connectWithRetry(ctx context.Context, url string, logger *zap.Logger) (*pgxpool.Pool, error) {
	var lastErr error
	for i := 0; i < 30; i++ {
		pool, err := database.NewPool(ctx, url)
		if err == nil {
			return pool, nil
		}
		lastErr = err
		logger.Warn("esperando a la base de datos…", zap.Int("intento", i+1))
		time.Sleep(2 * time.Second)
	}
	return nil, fmt.Errorf("no se pudo conectar a la base de datos: %w", lastErr)
}

// facturadorCxP adapta el servicio de CxP al puerto que espera Inventario para la consignación.
//
// La provisión nace como documento de tipo INTERNO y no CXP, y es deliberado:
//
//   - INTERNO es «vía expresa» en CxP (no exige clave de Hacienda, no se enruta a un departamento y
//     no pasa por la validación de área). Una provisión que generó el sistema no tiene una factura
//     electrónica detrás todavía, y enrutarla a un departamento la trancaría: hay 14 departamentos
//     activos y solo 2 tienen validador asignado.
//   - Es el mismo camino que ya usa la reposición de caja chica con REINTEGRO.
//
// La clave sí va explícita aunque INTERNO no la exija: es la llave de idempotencia. CxP tiene un
// UNIQUE por (empresa, clave), así que un segundo intento de facturar la misma unidad choca contra
// el duplicado en vez de crear otra cuenta por pagar. Si se dejara vacía, CxP la generaría aleatoria
// y cada reintento fabricaría una factura nueva por el mismo cofre.
type facturadorCxP struct{ svc *cxp.Service }

func (f facturadorCxP) CrearProvisionConsignacion(ctx context.Context, empresaID string, fc inventario.FacturaConsignacion, usuarioID string) (string, string, error) {
	// El monto es el costo de la unidad TAL CUAL, sin IVA: decisión del Director Financiero. Se le
	// paga el número que la bodega tecleó al recibir el cofre; el IVA lo trae la factura real del
	// proveedor cuando llega y se concilia.
	total, err := decimal.NewFromString(fc.TotalCRC)
	if err != nil {
		return "", "", fmt.Errorf("provisión de consignación: monto %q inválido: %w", fc.TotalCRC, err)
	}
	doc, err := f.svc.CrearDocumento(ctx, empresaID, cxp.DocumentoInput{
		ProveedorID:  fc.ProveedorID,
		Clave:        fc.Clave,
		Consecutivo:  fc.Consecutivo,
		FechaEmision: fc.Fecha,
		Moneda:       "CRC",
		Total:        total,
		Descripcion:  fc.Descripcion,
		Tipo:         cxp.TipoInterno,
		// EL candado contra el doble pago. La provisión NO se paga: se reemplaza por la factura
		// electrónica real del proveedor cuando llega. Sin esta marca recorría el camino completo
		// hasta el archivo del banco —es vía expresa, se aprueba directo desde RECIBIDO— y si alguien
		// la pagaba antes de conciliarla, después entraba la factura verdadera y se pagaba el mismo
		// cofre dos veces.
		BloqueadoParaPago: true,
		BloqueoMotivo: "provisión de consignación: no se paga, se reemplaza por la factura del " +
			"proveedor desde Inventario › Consignación",
	}, usuarioID)
	if err != nil {
		return "", "", errorDeCxP(err)
	}
	return doc.ID, doc.Consecutivo, nil
}

func (f facturadorCxP) AnularProvision(ctx context.Context, empresaID, documentoID, motivo, usuarioID string) error {
	_, err := f.svc.AnularConMotivo(ctx, empresaID, documentoID, motivo, usuarioID)
	return errorDeCxP(err)
}

func (f facturadorCxP) DocumentoPorID(ctx context.Context, empresaID, documentoID string) (inventario.DocumentoCxP, error) {
	// DocumentoPorID de CxP ya filtra por empresa_id: un documento de otra empresa vuelve como «no
	// encontrado». Eso es lo que impide enlazar una unidad a la factura de otra empresa.
	d, err := f.svc.DocumentoPorID(ctx, empresaID, documentoID)
	if err != nil {
		return inventario.DocumentoCxP{}, errorDeDocumentoReal(err)
	}
	return inventario.DocumentoCxP{
		ID: d.ID, Consecutivo: d.Consecutivo, ProveedorID: d.ProveedorID,
		Estado: d.Estado, TotalCRC: d.TotalCRC,
	}, nil
}

func (f facturadorCxP) ProvisionPorClave(ctx context.Context, empresaID, clave string) (inventario.DocumentoCxP, error) {
	d, err := f.svc.DocumentoPorClave(ctx, empresaID, clave)
	if err != nil {
		return inventario.DocumentoCxP{}, errorDeDocumentoReal(err)
	}
	return inventario.DocumentoCxP{
		ID: d.ID, Consecutivo: d.Consecutivo, ProveedorID: d.ProveedorID,
		Estado: d.Estado, TotalCRC: d.TotalCRC,
	}, nil
}

// errorDeCxP traduce los rechazos de CxP al vocabulario de inventario.
//
// Es la razón de ser del adaptador. Sin esta traducción, un error del paquete cxp llega al switch de
// responder() de inventario, que no lo reconoce, y sale como 500 «error interno»: el usuario no puede
// distinguir «esa factura ya existe» —que es el guardarraíl de idempotencia funcionando— de un
// servidor caído, así que reintenta lo mismo. Es la quinta vez que este proyecto se topa con esta
// misma clase de defecto, y por eso la traducción vive acá y no en cada llamador.
func errorDeCxP(err error) error {
	switch {
	case err == nil:
		return nil
	case errors.Is(err, cxp.ErrDocumentoDuplicado):
		return inventario.ErrProvisionYaExisteEnCxP
	case errors.Is(err, cxp.ErrProveedorNoEncontrado):
		return fmt.Errorf("%w: el proveedor de la unidad no existe en cuentas por pagar", inventario.ErrCxPRechazo)
	case errors.Is(err, cxp.ErrDocumentoNoEncontrado):
		return inventario.ErrDocumentoRealNoEncontrado
	case errors.Is(err, cxp.ErrTransicionInvalida):
		return fmt.Errorf("%w: el estado de esa factura no permite anularla desde acá; revisala en cuentas por pagar", inventario.ErrCxPRechazo)
	default:
		// Cualquier otro rechazo viaja con su mensaje: es mejor mostrar el texto de CxP que esconderlo
		// detrás de «error interno».
		return fmt.Errorf("%w: %v", inventario.ErrCxPRechazo, err)
	}
}

// errorDeDocumentoReal traduce la búsqueda de un documento. «No encontrado» incluye el caso de que
// sea de otra empresa, porque la consulta de CxP filtra por empresa_id.
func errorDeDocumentoReal(err error) error {
	if errors.Is(err, cxp.ErrDocumentoNoEncontrado) {
		return inventario.ErrDocumentoRealNoEncontrado
	}
	return errorDeCxP(err)
}

// conciliadorCxP adapta el servicio de CxP al puerto que espera Bancos. Cada módulo declara su
// propio tipo de resultado para no depender del otro; acá se traduce en un solo lugar.
type conciliadorCxP struct{ svc *cxp.Service }

func (c conciliadorCxP) PrefijoHuella() string { return c.svc.PrefijoHuella() }

func (c conciliadorCxP) ConciliarHuella(ctx context.Context, empresaID, descripcion, montoBanco, usuarioID string) (bancos.ResultadoHuella, error) {
	r, err := c.svc.ConciliarHuella(ctx, empresaID, descripcion, montoBanco, usuarioID)
	if err != nil {
		return bancos.ResultadoHuella{}, err
	}
	return bancos.ResultadoHuella{
		Veredicto: r.Veredicto, Huella: r.Huella,
		DocumentoID: r.DocumentoID, Consecutivo: r.Consecutivo, Proveedor: r.Proveedor,
		ConceptoID: r.ConceptoID, ClasificacionID: r.ClasificacionID,
		MontoEsperado: r.MontoEsperado, MontoBanco: r.MontoBanco,
	}, nil
}
