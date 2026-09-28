// Package config carga la configuración del servicio desde variables de entorno.
package config

import (
	"fmt"
	"os"
	"strings"
	"time"
)

// Config agrupa los parámetros de arranque del backend GPVDP.
type Config struct {
	Env          string
	Port         string
	DatabaseURL  string
	JWTSecret    string
	AccessTTL    time.Duration
	RefreshTTL   time.Duration
	CORSOrigins  []string
	SeedOnStart  bool
	SeedEmail    string
	SeedPassword string
	// CierreBloqueante: si true, no se cierra un período con movimientos "No identificado" (RN-22).
	CierreBloqueante bool
	// SMTP para el comprobante de pago al proveedor y las notificaciones de RRHH.
	//
	// En dev apunta a MailHog (mailhog:1025, sin autenticación) para ver lo que sale sin mandarle
	// nada a nadie. En producción hace falta un servidor real: `SMTP_USER` y `SMTP_PASS` vacíos
	// significan «sin autenticación», que es lo que ningún proveedor real acepta —y por eso el
	// envío daba error interno en producción con el valor por defecto—.
	//
	// Puerto: 587 (STARTTLS). El 465 (TLS implícito) no está soportado.
	SMTPAddr string
	SMTPFrom string
	SMTPUser string
	SMTPPass string
	// CifradoSecret es la clave con la que se cifran los secretos que el sistema tiene que volver a
	// USAR —hoy, la contraseña del correo saliente de cada empresa (mig 0084)—. Vive acá, en el
	// proceso, y NO en la base: un respaldo de Postgres que alguien se lleve trae texto inútil.
	//
	// SIN VALOR POR DEFECTO, igual que JWT_SECRET: una clave de fábrica en el código no cifraría
	// nada.
	//
	// Pero, a diferencia de JWT_SECRET, NO es obligatoria y `Load` no aborta si falta. Hay
	// instalaciones corriendo hoy sin esta variable, y hacerla obligatoria convertiría una mejora
	// del correo en la caída del ERP entero: el instalador conserva los `.env` que ya existen. Sin
	// ella, el correo sigue saliendo por el SMTP global (las cuatro SMTP_* de arriba) y lo único
	// que no se puede es guardar la contraseña de una empresa.
	//
	// La VALIDACIÓN del contenido (largo mínimo, que no sea una frase repetida) vive en un solo
	// lugar, `shared.NewCifrador`, que es quien la usa; acá solo se lee. El fallo se cobra donde se
	// puede leer y arreglar —422 nombrando CIFRADO_SECRET al guardar una contraseña— y nunca como
	// un error interno ni un panic al arrancar. `CifradoConfigurado` dice si al menos viene algo.
	//
	// ⚠ SI SE PIERDE, LAS CONTRASEÑAS GUARDADAS SON IRRECUPERABLES: hay que volver a escribir la de
	// cada empresa. El aviso está en el `.env` que genera deploy/instalar-vps.sh y en
	// deploy/LEEME-VPS.md.
	CifradoSecret string
	// BCCR: auto-sync del tipo de cambio (§22/§23). Desactivado por defecto: sin
	// credenciales (correo+token registrados en el BCCR) el sync no funciona y el
	// motor sigue siendo 100% manual. El indicador por defecto es 318 (venta) —
	// decisión del DF pendiente de confirmar; se cambia por env sin recompilar.
	BCCRSyncEnabled bool
	BCCRWSURL       string
	BCCREmail       string
	BCCRToken       string
	BCCRIndicador   string
	BCCRTimeout     time.Duration
}

// Load lee la configuración del entorno y valida lo obligatorio.
func Load() (Config, error) {
	cfg := Config{
		Env:              getenv("APP_ENV", "development"),
		Port:             getenv("PORT", "8080"),
		DatabaseURL:      os.Getenv("DATABASE_URL"),
		JWTSecret:        os.Getenv("JWT_SECRET"),
		AccessTTL:        getdur("ACCESS_TTL", 15*time.Minute),
		RefreshTTL:       getdur("REFRESH_TTL", 720*time.Hour),
		CORSOrigins:      splitList(getenv("CORS_ORIGINS", "http://localhost:5173")),
		SeedOnStart:      getbool("SEED_ON_START", false),
		SeedEmail:        getenv("SEED_ADMIN_EMAIL", "admin@gpvdp.local"),
		SeedPassword:     getenv("SEED_ADMIN_PASSWORD", "admin1234"),
		CierreBloqueante: getbool("CIERRE_PERIODO_BLOQUEANTE", true),
		SMTPAddr:         getenv("SMTP_ADDR", "mailhog:1025"),
		SMTPFrom:         getenv("SMTP_FROM", "cxp@valledepazcr.com"),
		SMTPUser:         os.Getenv("SMTP_USER"),
		SMTPPass:         os.Getenv("SMTP_PASS"),
		CifradoSecret:    strings.TrimSpace(os.Getenv("CIFRADO_SECRET")),
		BCCRSyncEnabled:  getbool("BCCR_SYNC_ENABLED", false),
		BCCRWSURL:        getenv("BCCR_WS_URL", "https://gee.bccr.fi.cr/Indicadores/Suministro/SW/wsindicadoreseconomicos.asmx/ObtenerIndicadoresEconomicosXML"),
		BCCREmail:        os.Getenv("BCCR_EMAIL"),
		BCCRToken:        os.Getenv("BCCR_TOKEN"),
		BCCRIndicador:    getenv("BCCR_INDICADOR", "318"),
		BCCRTimeout:      getdur("BCCR_TIMEOUT", 15*time.Second),
	}
	if cfg.DatabaseURL == "" {
		return cfg, fmt.Errorf("config: DATABASE_URL es obligatorio")
	}
	if cfg.JWTSecret == "" {
		return cfg, fmt.Errorf("config: JWT_SECRET es obligatorio")
	}
	// Los access tokens se firman con HS256 (clave simétrica). Una clave corta se crackea
	// offline y permite forjar cualquier token (incluido rol ADMIN). Se exige ≥ 32 bytes.
	if len(cfg.JWTSecret) < 32 {
		return cfg, fmt.Errorf("config: JWT_SECRET debe tener al menos 32 caracteres (tiene %d)", len(cfg.JWTSecret))
	}
	// El MailHog por defecto vale SOLO fuera de producción.
	//
	// En el servidor ese host no existe, así que el default convertía «falta configurar el correo»
	// en un «error interno» al mandarle el comprobante al proveedor —pasó el 9 de setiembre de
	// 2026—. Vacío, quien envía puede decir exactamente qué falta.
	//
	// Se limpia acá y no con el default de `getenv` porque una variable presente pero VACÍA —como
	// la deja el compose de producción— cae igual en el default.
	if cfg.IsProduction() && strings.TrimSpace(os.Getenv("SMTP_ADDR")) == "" {
		cfg.SMTPAddr = ""
	}
	return cfg, nil
}

// IsProduction indica si el servicio corre en modo producción.
func (c Config) IsProduction() bool { return c.Env == "production" }

// CifradoConfigurado dice si la instalación trae CIFRADO_SECRET.
//
// Sirve para que el arranque avise —con un WARN, nunca abortando— que el correo por empresa queda
// deshabilitado, y para que quien lea el log sepa por qué guardar una contraseña va a responder
// 422. Que el valor además SIRVA lo decide `shared.NewCifrador`.
func (c Config) CifradoConfigurado() bool { return c.CifradoSecret != "" }

func getenv(key, def string) string {
	if v := strings.TrimSpace(os.Getenv(key)); v != "" {
		return v
	}
	return def
}

func getdur(key string, def time.Duration) time.Duration {
	v := strings.TrimSpace(os.Getenv(key))
	if v == "" {
		return def
	}
	d, err := time.ParseDuration(v)
	if err != nil {
		return def
	}
	return d
}

func getbool(key string, def bool) bool {
	v := strings.ToLower(strings.TrimSpace(os.Getenv(key)))
	switch v {
	case "1", "true", "yes", "y", "on":
		return true
	case "0", "false", "no", "n", "off":
		return false
	default:
		return def
	}
}

func splitList(v string) []string {
	parts := strings.Split(v, ",")
	out := make([]string, 0, len(parts))
	for _, p := range parts {
		if p = strings.TrimSpace(p); p != "" {
			out = append(out, p)
		}
	}
	return out
}
