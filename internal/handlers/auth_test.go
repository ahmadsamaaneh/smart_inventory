package handlers_test

import (
	"bytes"
	"encoding/json"
	"net/http"
	"net/http/httptest"
	"path/filepath"
	"testing"

	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/ahmad/smart-inventory/internal/server"
	"github.com/gin-gonic/gin"
	"github.com/glebarez/sqlite"
	"gorm.io/gorm"
	"gorm.io/gorm/logger"
)

func setupTestServer(t *testing.T) (*gin.Engine, *gorm.DB, *auth.Manager) {
	t.Helper()
	gin.SetMode(gin.TestMode)

	dsn := filepath.Join(t.TempDir(), "test.db")
	db, err := gorm.Open(sqlite.Open(dsn), &gorm.Config{
		Logger: logger.Default.LogMode(logger.Silent),
	})
	if err != nil {
		t.Fatalf("open db: %v", err)
	}
	if err := db.AutoMigrate(
		&models.User{}, &models.Product{}, &models.Order{},
		&models.OrderItem{}, &models.StockMovement{},
	); err != nil {
		t.Fatalf("migrate: %v", err)
	}

	jwtMgr := auth.NewManager("test-secret", 1)
	r := server.NewRouter(db, jwtMgr)
	return r, db, jwtMgr
}

func doJSON(t *testing.T, r http.Handler, method, path string, body any, token string) *httptest.ResponseRecorder {
	t.Helper()
	var buf bytes.Buffer
	if body != nil {
		_ = json.NewEncoder(&buf).Encode(body)
	}
	req := httptest.NewRequest(method, path, &buf)
	req.Header.Set("Content-Type", "application/json")
	if token != "" {
		req.Header.Set("Authorization", "Bearer "+token)
	}
	w := httptest.NewRecorder()
	r.ServeHTTP(w, req)
	return w
}

func decode(t *testing.T, w *httptest.ResponseRecorder, dst any) {
	t.Helper()
	if err := json.Unmarshal(w.Body.Bytes(), dst); err != nil {
		t.Fatalf("decode body %q: %v", w.Body.String(), err)
	}
}

func TestAuth_RegisterLoginAndProtectedRoute(t *testing.T) {
	r, _, _ := setupTestServer(t)

	// register a new user
	regBody := map[string]string{
		"email":    "alice@example.com",
		"password": "Sup3rSecret!",
		"name":     "Alice",
	}
	w := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", regBody, "")
	if w.Code != http.StatusCreated {
		t.Fatalf("register: want 201, got %d body=%s", w.Code, w.Body.String())
	}
	var reg struct {
		Token string       `json:"token"`
		User  *models.User `json:"user"`
	}
	decode(t, w, &reg)
	if reg.Token == "" || reg.User == nil || reg.User.Email != "alice@example.com" {
		t.Fatalf("register response invalid: %+v", reg)
	}

	// same email again -> conflict
	w = doJSON(t, r, http.MethodPost, "/api/v1/auth/register", regBody, "")
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate register: want 409, got %d", w.Code)
	}

	// wrong password
	w = doJSON(t, r, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "alice@example.com", "password": "wrong",
	}, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("wrong-password login: want 401, got %d", w.Code)
	}

	// correct login
	w = doJSON(t, r, http.MethodPost, "/api/v1/auth/login", map[string]string{
		"email": "alice@example.com", "password": "Sup3rSecret!",
	}, "")
	if w.Code != http.StatusOK {
		t.Fatalf("login: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var login struct{ Token string }
	decode(t, w, &login)
	if login.Token == "" {
		t.Fatal("expected non-empty token from login")
	}

	// /me without token
	w = doJSON(t, r, http.MethodGet, "/api/v1/auth/me", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("no-token /me: want 401, got %d", w.Code)
	}

	// /me with garbage token
	w = doJSON(t, r, http.MethodGet, "/api/v1/auth/me", nil, "not.a.jwt")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("bad-token /me: want 401, got %d", w.Code)
	}

	// /me with real token
	w = doJSON(t, r, http.MethodGet, "/api/v1/auth/me", nil, login.Token)
	if w.Code != http.StatusOK {
		t.Fatalf("good-token /me: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var me models.User
	decode(t, w, &me)
	if me.Email != "alice@example.com" {
		t.Fatalf("/me returned wrong user: %+v", me)
	}
}

func TestAuth_RegisterValidation(t *testing.T) {
	r, _, _ := setupTestServer(t)

	cases := []struct {
		name string
		body map[string]string
	}{
		{"missing email", map[string]string{"password": "Sup3rSecret!", "name": "X"}},
		{"bad email", map[string]string{"email": "nope", "password": "Sup3rSecret!", "name": "X"}},
		{"short password", map[string]string{"email": "x@y.com", "password": "short", "name": "X"}},
		{"missing name", map[string]string{"email": "x@y.com", "password": "Sup3rSecret!"}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/api/v1/auth/register", tc.body, "")
			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}
