package handlers_test

import (
	"net/http"
	"testing"

	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/models"
)

func TestInventory_AddRemoveAndGet(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	// admin for making inventory changes
	hash, _ := auth.HashPassword("Admin123!")
	admin := &models.User{Email: "i@x.com", PasswordHash: hash, Name: "I", Role: models.RoleAdmin}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	tok, _, _ := jwtMgr.Issue(admin)

	// product starts with 10 in stock
	p := &models.Product{
		SKU: "INV-1", Name: "Tile", Material: "marble", Size: "60x60",
		ThicknessMM: 10, PriceCents: 4999, Stock: 10, LowStockAt: 3, Active: true,
	}
	if err := db.Create(p).Error; err != nil {
		t.Fatalf("seed product: %v", err)
	}

	// should show 10 in stock
	w := doJSON(t, r, http.MethodGet, "/api/v1/inventory/"+itoa(p.ID), nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("get inventory: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	var view struct {
		ProductID uint `json:"product_id"`
		Quantity  int  `json:"quantity"`
		LowStock  bool `json:"low_stock"`
	}
	decode(t, w, &view)
	if view.ProductID != p.ID || view.Quantity != 10 {
		t.Fatalf("unexpected inventory view: %+v", view)
	}

	// add 5 -> 15
	w = doJSON(t, r, http.MethodPost, "/api/v1/inventory/add",
		map[string]any{"product_id": p.ID, "quantity": 5, "reason": "restock"}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("add: want 200, got %d body=%s", w.Code, w.Body.String())
	}
	decode(t, w, &view)
	if view.Quantity != 15 {
		t.Fatalf("after add: want 15, got %d", view.Quantity)
	}

	// remove 4 -> 11
	w = doJSON(t, r, http.MethodPost, "/api/v1/inventory/remove",
		map[string]any{"product_id": p.ID, "quantity": 4}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("remove: want 200, got %d", w.Code)
	}
	decode(t, w, &view)
	if view.Quantity != 11 {
		t.Fatalf("after remove: want 11, got %d", view.Quantity)
	}

	// double check it persisted
	w = doJSON(t, r, http.MethodGet, "/api/v1/inventory/"+itoa(p.ID), nil, tok)
	decode(t, w, &view)
	if view.Quantity != 11 {
		t.Fatalf("persisted: want 11, got %d", view.Quantity)
	}
}

func TestInventory_CannotGoNegative(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	hash, _ := auth.HashPassword("x")
	admin := &models.User{Email: "n@x.com", PasswordHash: hash, Name: "N", Role: models.RoleAdmin}
	_ = db.Create(admin).Error
	tok, _, _ := jwtMgr.Issue(admin)

	p := &models.Product{
		SKU: "INV-2", Name: "Slab", Material: "granite", Size: "120x60",
		ThicknessMM: 20, PriceCents: 9999, Stock: 3, LowStockAt: 1, Active: true,
	}
	_ = db.Create(p).Error

	// removing 5 from stock of 3 should fail
	w := doJSON(t, r, http.MethodPost, "/api/v1/inventory/remove",
		map[string]any{"product_id": p.ID, "quantity": 5}, tok)
	if w.Code != http.StatusConflict {
		t.Fatalf("over-remove: want 409, got %d body=%s", w.Code, w.Body.String())
	}

	var fresh models.Product
	if err := db.First(&fresh, p.ID).Error; err != nil {
		t.Fatalf("reload: %v", err)
	}
	if fresh.Stock != 3 {
		t.Fatalf("stock changed despite rejection: %d", fresh.Stock)
	}

	// but taking exactly 3 from 3 should work (-> 0)
	w = doJSON(t, r, http.MethodPost, "/api/v1/inventory/remove",
		map[string]any{"product_id": p.ID, "quantity": 3}, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("zero-out: want 200, got %d", w.Code)
	}

	// now any further removal should fail
	w = doJSON(t, r, http.MethodPost, "/api/v1/inventory/remove",
		map[string]any{"product_id": p.ID, "quantity": 1}, tok)
	if w.Code != http.StatusConflict {
		t.Fatalf("post-zero remove: want 409, got %d", w.Code)
	}
}

func TestInventory_ValidationAndAuth(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	hash, _ := auth.HashPassword("x")
	admin := &models.User{Email: "v@x.com", PasswordHash: hash, Name: "V", Role: models.RoleAdmin}
	cust := &models.User{Email: "vc@x.com", PasswordHash: hash, Name: "VC", Role: models.RoleCustomer}
	_ = db.Create(admin).Error
	_ = db.Create(cust).Error
	atok, _, _ := jwtMgr.Issue(admin)
	ctok, _, _ := jwtMgr.Issue(cust)

	// missing product_id
	w := doJSON(t, r, http.MethodPost, "/api/v1/inventory/add", map[string]any{"quantity": 1}, atok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("missing product_id: want 400, got %d", w.Code)
	}
	// quantity must be > 0
	w = doJSON(t, r, http.MethodPost, "/api/v1/inventory/add", map[string]any{"product_id": 1, "quantity": 0}, atok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("zero quantity: want 400, got %d", w.Code)
	}
	w = doJSON(t, r, http.MethodPost, "/api/v1/inventory/remove", map[string]any{"product_id": 1, "quantity": -2}, atok)
	if w.Code != http.StatusBadRequest {
		t.Fatalf("negative quantity: want 400, got %d", w.Code)
	}

	// unknown product
	w = doJSON(t, r, http.MethodPost, "/api/v1/inventory/add", map[string]any{"product_id": 999, "quantity": 1}, atok)
	if w.Code != http.StatusNotFound {
		t.Fatalf("unknown product add: want 404, got %d", w.Code)
	}

	// customer can't touch stock
	w = doJSON(t, r, http.MethodPost, "/api/v1/inventory/add", map[string]any{"product_id": 1, "quantity": 1}, ctok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("customer add: want 403, got %d", w.Code)
	}
	// anon can't read inventory either
	w = doJSON(t, r, http.MethodGet, "/api/v1/inventory/1", nil, "")
	if w.Code != http.StatusUnauthorized {
		t.Fatalf("anon get: want 401, got %d", w.Code)
	}
}
