package auth

import (
	"context"
	"errors"
	"strconv"
	"testing"
	"time"
)

// ---- fake repository en memoria ----

type fakeRepo struct {
	byEmail     map[string]Usuario
	byID        map[string]Usuario
	memberships map[string][]Membership
	refreshHash map[string]RefreshRecord
	refreshID   map[string]RefreshRecord
	seq         int
}

func newFakeRepo() *fakeRepo {
	return &fakeRepo{
		byEmail:     map[string]Usuario{},
		byID:        map[string]Usuario{},
		memberships: map[string][]Membership{},
		refreshHash: map[string]RefreshRecord{},
		refreshID:   map[string]RefreshRecord{},
	}
}

func (f *fakeRepo) addUsuario(u Usuario, ms []Membership) {
	f.byEmail[u.Email] = u
	f.byID[u.ID] = u
	f.memberships[u.ID] = ms
}

func (f *fakeRepo) UsuarioByEmail(_ context.Context, email string) (Usuario, error) {
	u, ok := f.byEmail[email]
	if !ok {
		return Usuario{}, ErrCredenciales
	}
	return u, nil
}

func (f *fakeRepo) UsuarioByID(_ context.Context, id string) (Usuario, error) {
	u, ok := f.byID[id]
	if !ok {
		return Usuario{}, ErrCredenciales
	}
	return u, nil
}

func (f *fakeRepo) ActualizarPassword(_ context.Context, usuarioID, hash string, debeCambiar bool) error {
	if u, ok := f.byID[usuarioID]; ok {
		u.PasswordHash = hash
		u.DebeCambiarPassword = debeCambiar
		f.byID[usuarioID] = u
		f.byEmail[u.Email] = u
	}
	return nil
}

func (f *fakeRepo) Memberships(_ context.Context, usuarioID string) ([]Membership, error) {
	return f.memberships[usuarioID], nil
}

func (f *fakeRepo) Membership(_ context.Context, usuarioID, empresaID string) (Membership, error) {
	for _, m := range f.memberships[usuarioID] {
		if m.EmpresaID == empresaID {
			return m, nil
		}
	}
	return Membership{}, ErrSinAcceso
}

func (f *fakeRepo) CrearRefresh(_ context.Context, usuarioID, tokenHash string, expira time.Time) error {
	f.seq++
	rec := RefreshRecord{ID: strconv.Itoa(f.seq), UsuarioID: usuarioID, ExpiraEn: expira}
	f.refreshHash[tokenHash] = rec
	f.refreshID[rec.ID] = rec
	return nil
}

func (f *fakeRepo) RefreshPorHash(_ context.Context, tokenHash string) (RefreshRecord, error) {
	rec, ok := f.refreshHash[tokenHash]
	if !ok {
		return RefreshRecord{}, ErrRefreshInvalido
	}
	return rec, nil
}

func (f *fakeRepo) RevocarRefresh(_ context.Context, id string) (int64, error) {
	for h, rec := range f.refreshHash {
		if rec.ID == id {
			if rec.Revocado {
				return 0, nil
			}
			rec.Revocado = true
			f.refreshHash[h] = rec
			return 1, nil
		}
	}
	return 0, nil
}

func (f *fakeRepo) RevocarSesionesDeUsuario(_ context.Context, usuarioID string) (int64, error) {
	var n int64
	for h, rec := range f.refreshHash {
		if rec.UsuarioID == usuarioID && !rec.Revocado {
			rec.Revocado = true
			f.refreshHash[h] = rec
			n++
		}
	}
	return n, nil
}

func (f *fakeRepo) RegistrarIntentoFallido(_ context.Context, usuarioID string, umbral int, ventana time.Duration) (int, *time.Time, error) {
	u, ok := f.byID[usuarioID]
	if !ok {
		return 0, nil, ErrCredenciales
	}
	u.IntentosFallidos++
	if u.IntentosFallidos >= umbral {
		t := time.Now().Add(ventana)
		u.BloqueadoHasta = &t
	}
	f.byID[usuarioID] = u
	f.byEmail[u.Email] = u
	return u.IntentosFallidos, u.BloqueadoHasta, nil
}

func (f *fakeRepo) LimpiarIntentos(_ context.Context, usuarioID string) error {
	if u, ok := f.byID[usuarioID]; ok {
		u.IntentosFallidos = 0
		u.BloqueadoHasta = nil
		f.byID[usuarioID] = u
		f.byEmail[u.Email] = u
	}
	return nil
}

// ---- helpers ----

func nuevoServicioDePrueba(t *testing.T) (*Service, *fakeRepo) {
	t.Helper()
	repo := newFakeRepo()
	hash, err := HashPassword("secreto")
	if err != nil {
		t.Fatalf("HashPassword: %v", err)
	}
	repo.addUsuario(
		Usuario{ID: "u1", Nombre: "Ana", Email: "ana@vdp.com", PasswordHash: hash, Activo: true},
		[]Membership{{EmpresaID: "emp-vdp", EmpresaNombre: "Valle de Paz", RolID: "r1", RolCodigo: "ADMIN"}},
	)
	return NewService(repo, "secret-test", 15*time.Minute, 24*time.Hour), repo
}

// ---- tests ----

func TestLogin(t *testing.T) {
	svc, _ := nuevoServicioDePrueba(t)
	ctx := context.Background()

	t.Run("credenciales válidas emiten tokens sin empresa", func(t *testing.T) {
		res, err := svc.Login(ctx, "ana@vdp.com", "secreto")
		if err != nil {
			t.Fatalf("Login: %v", err)
		}
		if res.AccessToken == "" || res.RefreshToken == "" {
			t.Fatal("se esperaban access y refresh token")
		}
		claims, err := ParseAccessToken("secret-test", res.AccessToken)
		if err != nil {
			t.Fatalf("parse access: %v", err)
		}
		if claims.EmpresaID != "" {
			t.Errorf("el token de login no debe traer empresa, trae %q", claims.EmpresaID)
		}
		if len(res.Empresas) != 1 {
			t.Errorf("empresas = %d, quería 1", len(res.Empresas))
		}
	})

	t.Run("contraseña incorrecta", func(t *testing.T) {
		if _, err := svc.Login(ctx, "ana@vdp.com", "malo"); !errors.Is(err, ErrCredenciales) {
			t.Errorf("err = %v, quería ErrCredenciales", err)
		}
	})

	t.Run("email inexistente", func(t *testing.T) {
		if _, err := svc.Login(ctx, "nadie@vdp.com", "x"); !errors.Is(err, ErrCredenciales) {
			t.Errorf("err = %v, quería ErrCredenciales", err)
		}
	})
}

func TestSelectEmpresa(t *testing.T) {
	svc, _ := nuevoServicioDePrueba(t)
	ctx := context.Background()

	t.Run("con pertenencia scopea el token", func(t *testing.T) {
		access, m, err := svc.SelectEmpresa(ctx, "u1", "emp-vdp")
		if err != nil {
			t.Fatalf("SelectEmpresa: %v", err)
		}
		if m.RolCodigo != "ADMIN" {
			t.Errorf("rol = %q", m.RolCodigo)
		}
		claims, err := ParseAccessToken("secret-test", access)
		if err != nil {
			t.Fatalf("parse: %v", err)
		}
		if claims.EmpresaID != "emp-vdp" || claims.Rol != "ADMIN" {
			t.Errorf("claims empresa/rol = %q/%q", claims.EmpresaID, claims.Rol)
		}
	})

	t.Run("sin pertenencia es rechazado", func(t *testing.T) {
		if _, _, err := svc.SelectEmpresa(ctx, "u1", "empresa-ajena"); !errors.Is(err, ErrSinAcceso) {
			t.Errorf("err = %v, quería ErrSinAcceso", err)
		}
	})
}

func TestRefreshRotacion(t *testing.T) {
	svc, _ := nuevoServicioDePrueba(t)
	ctx := context.Background()

	res, err := svc.Login(ctx, "ana@vdp.com", "secreto")
	if err != nil {
		t.Fatalf("Login: %v", err)
	}

	access2, refresh2, err := svc.Refresh(ctx, res.RefreshToken, "emp-vdp")
	if err != nil {
		t.Fatalf("Refresh: %v", err)
	}
	if refresh2 == res.RefreshToken {
		t.Error("el refresh token debería rotar")
	}
	claims, err := ParseAccessToken("secret-test", access2)
	if err != nil {
		t.Fatalf("parse: %v", err)
	}
	if claims.EmpresaID != "emp-vdp" {
		t.Errorf("el refresh con empresa debe scopear el token, empresa = %q", claims.EmpresaID)
	}

	// El refresh viejo quedó revocado.
	if _, _, err := svc.Refresh(ctx, res.RefreshToken, ""); !errors.Is(err, ErrRefreshInvalido) {
		t.Errorf("el refresh viejo debe ser inválido tras la rotación, err = %v", err)
	}
}

func TestRefreshUsuarioInactivo(t *testing.T) {
	svc, repo := nuevoServicioDePrueba(t)
	ctx := context.Background()

	// Usuario inactivo con un refresh token vigente (p. ej. desactivado tras loguearse).
	repo.addUsuario(Usuario{ID: "u2", Nombre: "Ex", Email: "ex@vdp.com", PasswordHash: "x", Activo: false}, nil)
	if err := repo.CrearRefresh(ctx, "u2", hashToken("tok-inactivo"), time.Now().Add(time.Hour)); err != nil {
		t.Fatalf("CrearRefresh: %v", err)
	}

	if _, _, err := svc.Refresh(ctx, "tok-inactivo", ""); !errors.Is(err, ErrUsuarioInactivo) {
		t.Errorf("Refresh de usuario inactivo: err = %v, quería ErrUsuarioInactivo", err)
	}
}

func TestSelectEmpresaUsuarioInactivo(t *testing.T) {
	svc, repo := nuevoServicioDePrueba(t)

	repo.addUsuario(
		Usuario{ID: "u3", Nombre: "Ex", Email: "ex3@vdp.com", PasswordHash: "x", Activo: false},
		[]Membership{{EmpresaID: "emp-x", EmpresaNombre: "X", RolID: "r1", RolCodigo: "ADMIN"}},
	)

	if _, _, err := svc.SelectEmpresa(context.Background(), "u3", "emp-x"); !errors.Is(err, ErrUsuarioInactivo) {
		t.Errorf("SelectEmpresa de usuario inactivo: err = %v, quería ErrUsuarioInactivo", err)
	}
}

// ---- tests de endurecimiento (hardening) ----

func TestLoginBloqueoPorFuerzaBruta(t *testing.T) {
	svc, _ := nuevoServicioDePrueba(t)
	ctx := context.Background()

	// Los primeros (umbral-1) fallos devuelven credenciales inválidas.
	for i := 1; i < maxIntentosLogin; i++ {
		if _, err := svc.Login(ctx, "ana@vdp.com", "malo"); !errors.Is(err, ErrCredenciales) {
			t.Fatalf("intento %d: err = %v, quería ErrCredenciales", i, err)
		}
	}
	// El fallo que alcanza el umbral bloquea la cuenta.
	if _, err := svc.Login(ctx, "ana@vdp.com", "malo"); !errors.Is(err, ErrCuentaBloqueada) {
		t.Fatalf("al alcanzar el umbral: err = %v, quería ErrCuentaBloqueada", err)
	}
	// Ya bloqueada, ni siquiera la contraseña correcta entra.
	if _, err := svc.Login(ctx, "ana@vdp.com", "secreto"); !errors.Is(err, ErrCuentaBloqueada) {
		t.Errorf("con contraseña correcta pero bloqueada: err = %v, quería ErrCuentaBloqueada", err)
	}
}

func TestLoginExitosoLimpiaIntentos(t *testing.T) {
	svc, repo := nuevoServicioDePrueba(t)
	ctx := context.Background()

	// Unos cuantos fallos por debajo del umbral.
	for i := 0; i < 3; i++ {
		_, _ = svc.Login(ctx, "ana@vdp.com", "malo")
	}
	// Un login correcto reinicia el contador.
	if _, err := svc.Login(ctx, "ana@vdp.com", "secreto"); err != nil {
		t.Fatalf("Login correcto: %v", err)
	}
	if u := repo.byID["u1"]; u.IntentosFallidos != 0 || u.BloqueadoHasta != nil {
		t.Errorf("tras login correcto: intentos=%d bloqueado=%v, quería 0/nil", u.IntentosFallidos, u.BloqueadoHasta)
	}
}

func TestLoginUsuarioInactivoNoRevelaEstado(t *testing.T) {
	svc, repo := nuevoServicioDePrueba(t)
	ctx := context.Background()

	u := repo.byID["u1"]
	u.Activo = false
	repo.byID["u1"] = u
	repo.byEmail[u.Email] = u

	// Anti-enumeración: un usuario inactivo devuelve el MISMO error que credenciales malas,
	// no un "usuario inactivo" que confirmaría que la cuenta existe.
	if _, err := svc.Login(ctx, "ana@vdp.com", "secreto"); !errors.Is(err, ErrCredenciales) {
		t.Errorf("usuario inactivo: err = %v, quería ErrCredenciales (no revelar estado)", err)
	}
}

func TestCambiarPasswordRevocaSesiones(t *testing.T) {
	svc, repo := nuevoServicioDePrueba(t)
	ctx := context.Background()

	// Abre dos sesiones (dos refresh tokens vigentes).
	if _, err := svc.Login(ctx, "ana@vdp.com", "secreto"); err != nil {
		t.Fatalf("login 1: %v", err)
	}
	if _, err := svc.Login(ctx, "ana@vdp.com", "secreto"); err != nil {
		t.Fatalf("login 2: %v", err)
	}
	vigentes := 0
	for _, rec := range repo.refreshHash {
		if rec.UsuarioID == "u1" && !rec.Revocado {
			vigentes++
		}
	}
	if vigentes < 2 {
		t.Fatalf("precondición: se esperaban ≥2 sesiones vigentes, hay %d", vigentes)
	}
	// Cambiar la contraseña revoca todas las sesiones abiertas.
	if err := svc.CambiarPassword(ctx, "u1", "secreto", "nuevaClave123"); err != nil {
		t.Fatalf("CambiarPassword: %v", err)
	}
	for _, rec := range repo.refreshHash {
		if rec.UsuarioID == "u1" && !rec.Revocado {
			t.Errorf("quedó una sesión vigente tras cambiar la contraseña")
		}
	}
}
