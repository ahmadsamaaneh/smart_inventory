package handlers_test

import (
	"net/http"
	"strconv"
	"testing"

	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/models"
)

func itoa(u uint) string { return strconv.FormatUint(uint64(u), 10) }

func TestProducts_FullCRUD(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	// seed admin directly in db
	hash, _ := auth.HashPassword("Admin123!")
	admin := &models.User{Email: "admin@x.com", PasswordHash: hash, Name: "Admin", Role: models.RoleAdmin}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	tok, _, err := jwtMgr.Issue(admin)
	if err != nil {
		t.Fatalf("issue admin token: %v", err)
	}

	valid := map[string]any{
		"sku":          "TILE-001",
		"name":         "Marble Tile",
		"description":  "Polished white marble",
		"category":     "tiles",
		"material":     "marble",
		"size":         "60x60",
		"thickness_mm": 10.0,
		"price_cents":  4999,
		"stock":        20,
		"low_stock_at": 3,
	}

	// CREATE
	w := doJSON(t, r, http.MethodPost, "/api/v1/products", valid, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d body=%s", w.Code, w.Body.String())
	}
	var created models.Product
	decode(t, w, &created)
	if created.ID == 0 || created.Material != "marble" || created.Size != "60x60" || created.ThicknessMM != 10.0 {
		t.Fatalf("created product missing fields: %+v", created)
	}

	// duplicate SKU should 409
	w = doJSON(t, r, http.MethodPost, "/api/v1/products", valid, tok)
	if w.Code != http.StatusConflict {
		t.Fatalf("duplicate SKU: want 409, got %d", w.Code)
	}

	// listing is public
	w = doJSON(t, r, http.MethodGet, "/api/v1/products", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
	var list struct {
		Items []models.Product `json:"items"`
		Total int64            `json:"total"`
	}
	decode(t, w, &list)
	if list.Total < 1 || len(list.Items) < 1 {
		t.Fatalf("list empty: %+v", list)
	}

	// filter by material
	w = doJSON(t, r, http.MethodGet, "/api/v1/products?material=marble", nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("list by material: want 200, got %d", w.Code)
	}
	decode(t, w, &list)
	if list.Total < 1 {
		t.Fatalf("expected at least one marble product")
	}

	// get by id
	w = doJSON(t, r, http.MethodGet, "/api/v1/products/"+itoa(created.ID), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get: want 200, got %d", w.Code)
	}

	// unknown id -> 404
	w = doJSON(t, r, http.MethodGet, "/api/v1/products/999999", nil, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("get unknown: want 404, got %d", w.Code)
	}

	// UPDATE
	upd := map[string]any{"price_cents": 5999, "thickness_mm": 12.0}
	w = doJSON(t, r, http.MethodPut, "/api/v1/products/"+itoa(created.ID), upd, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("update: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var after models.Product
	decode(t, w, &after)
	if after.PriceCents != 5999 || after.ThicknessMM != 12.0 {
		t.Fatalf("update did not persist: %+v", after)
	}

	// DELETE
	w = doJSON(t, r, http.MethodDelete, "/api/v1/products/"+itoa(created.ID), nil, tok)
	if w.Code != http.StatusNoContent {
		t.Fatalf("delete: want 204, got %d", w.Code)
	}
	w = doJSON(t, r, http.MethodGet, "/api/v1/products/"+itoa(created.ID), nil, "")
	if w.Code != http.StatusNotFound {
		t.Fatalf("after delete: want 404, got %d", w.Code)
	}
}

func TestProducts_ValidationErrors(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)
	hash, _ := auth.HashPassword("x")
	admin := &models.User{Email: "a@x.com", PasswordHash: hash, Name: "A", Role: models.RoleAdmin}
	_ = db.Create(admin).Error
	tok, _, _ := jwtMgr.Issue(admin)

	cases := []struct {
		name string
		body map[string]any
	}{
		{"missing sku", map[string]any{"name": "n", "material": "m", "size": "s", "thickness_mm": 1.0, "price_cents": 100}},
		{"missing name", map[string]any{"sku": "x", "material": "m", "size": "s", "thickness_mm": 1.0, "price_cents": 100}},
		{"missing material", map[string]any{"sku": "x", "name": "n", "size": "s", "thickness_mm": 1.0, "price_cents": 100}},
		{"missing size", map[string]any{"sku": "x", "name": "n", "material": "m", "thickness_mm": 1.0, "price_cents": 100}},
		{"thickness <= 0", map[string]any{"sku": "x", "name": "n", "material": "m", "size": "s", "thickness_mm": 0, "price_cents": 100}},
		{"missing price", map[string]any{"sku": "x", "name": "n", "material": "m", "size": "s", "thickness_mm": 1.0}},
	}
	for _, tc := range cases {
		t.Run(tc.name, func(t *testing.T) {
			w := doJSON(t, r, http.MethodPost, "/api/v1/products", tc.body, tok)
			if w.Code != http.StatusBadRequest {
				t.Fatalf("want 400, got %d body=%s", w.Code, w.Body.String())
			}
		})
	}
}

func TestProducts_RBAC(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	// customers can't create products
	hash, _ := auth.HashPassword("x")
	cust := &models.User{Email: "c@x.com", PasswordHash: hash, Name: "C", Role: models.RoleCustomer}
	_ = db.Create(cust).Error
	ctok, _, _ := jwtMgr.Issue(cust)

	body := map[string]any{
		"sku": "X1", "name": "n", "material": "m", "size": "s",
		"thickness_mm": 1.0, "price_cents": 100,
	}
	w := doJSON(t, r, http.MethodPost, "/api/v1/products", body, ctok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("customer create: want 403, got %d", w.Code)
	}

	// anon can't either
	w = doJSON(t, r, http.MethodPost, "/api/v1/products", body, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anon create: want 401, got %d", w.Code)
	}
}
