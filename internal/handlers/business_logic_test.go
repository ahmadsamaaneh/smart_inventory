package handlers_test

import (
	"net/http"
	"testing"

	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/models"
)

// Big integration test: order placement, stock deduction, rejection on
// insufficient stock, and the low-stock flag on product responses.
func TestBusinessLogic_StockGuardAndLowStockFlag(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	// set up a customer
	hash, _ := auth.HashPassword("Passw0rd!")
	cust := &models.User{Email: "buyer@bl.com", PasswordHash: hash, Name: "Buyer", Role: models.RoleCustomer}
	if err := db.Create(cust).Error; err != nil {
		t.Fatalf("seed customer: %v", err)
	}
	ctok, _, _ := jwtMgr.Issue(cust)

	// and an admin
	admin := &models.User{Email: "admin@bl.com", PasswordHash: hash, Name: "Admin", Role: models.RoleAdmin}
	if err := db.Create(admin).Error; err != nil {
		t.Fatalf("seed admin: %v", err)
	}
	atok, _, _ := jwtMgr.Issue(admin)

	// P1 starts well-stocked, we'll draw it down
	p1 := &models.Product{
		SKU: "BL-1", Name: "Watched Tile", Material: "marble", Size: "60x60",
		ThicknessMM: 10, PriceCents: 1000, Stock: 10, LowStockAt: 3, Active: true,
	}
	// P2 stays well-stocked (control group)
	p2 := &models.Product{
		SKU: "BL-2", Name: "Plenty Tile", Material: "granite", Size: "30x60",
		ThicknessMM: 12, PriceCents: 500, Stock: 50, LowStockAt: 5, Active: true,
	}
	if err := db.Create(p1).Error; err != nil {
		t.Fatalf("seed p1: %v", err)
	}
	if err := db.Create(p2).Error; err != nil {
		t.Fatalf("seed p2: %v", err)
	}

	// initially p1 is NOT low-stock
	w := doJSON(t, r, http.MethodGet, "/api/v1/products/"+itoa(p1.ID), nil, "")
	if w.Code != http.StatusOK {
		t.Fatalf("get p1: %d", w.Code)
	}
	var got models.Product
	decode(t, w, &got)
	if got.LowStock {
		t.Fatalf("expected p1 low_stock=false initially, got true (stock=%d, threshold=%d)", got.Stock, got.LowStockAt)
	}

	// over-order should fail and leave stock untouched
	w = doJSON(t, r, http.MethodPost, "/api/v1/orders",
		map[string]any{"items": []map[string]any{{"product_id": p1.ID, "quantity": 11}}}, ctok)
	if w.Code != http.StatusConflict {
		t.Fatalf("over-order: want 409, got %d body=%s", w.Code, w.Body.String())
	}
	var fresh models.Product
	if err := db.First(&fresh, p1.ID).Error; err != nil {
		t.Fatalf("reload p1: %v", err)
	}
	if fresh.Stock != 10 {
		t.Fatalf("stock changed despite rejection: %d", fresh.Stock)
	}

	// order 8 out of 10 -> stock drops to 2 which is <= threshold of 3
	w = doJSON(t, r, http.MethodPost, "/api/v1/orders",
		map[string]any{"items": []map[string]any{{"product_id": p1.ID, "quantity": 8}}}, ctok)
	if w.Code != http.StatusCreated {
		t.Fatalf("order: want 201, got %d body=%s", w.Code, w.Body.String())
	}

	// verify stock was actually deducted
	w = doJSON(t, r, http.MethodGet, "/api/v1/products/"+itoa(p1.ID), nil, "")
	decode(t, w, &got)
	if got.Stock != 2 {
		t.Fatalf("stock not deducted: want 2, got %d", got.Stock)
	}

	// low_stock flag should now be true
	if !got.LowStock {
		t.Fatalf("expected p1 low_stock=true after dropping to %d <= threshold %d", got.Stock, got.LowStockAt)
	}

	// low-stock report should include p1 but not p2
	w = doJSON(t, r, http.MethodGet, "/api/v1/inventory/low-stock", nil, atok)
	if w.Code != http.StatusOK {
		t.Fatalf("low-stock: %d", w.Code)
	}
	var rep struct {
		Items []models.Product `json:"items"`
		Count int              `json:"count"`
	}
	decode(t, w, &rep)

	hasP1, hasP2 := false, false
	for _, it := range rep.Items {
		if it.ID == p1.ID {
			hasP1 = true
			if !it.LowStock {
				t.Fatalf("p1 in low-stock report should also have low_stock=true: %+v", it)
			}
		}
		if it.ID == p2.ID {
			hasP2 = true
		}
	}
	if !hasP1 {
		t.Fatalf("low-stock report missing p1: %+v", rep)
	}
	if hasP2 {
		t.Fatalf("low-stock report should not include well-stocked p2: %+v", rep)
	}

	// drain p1 to zero, should still be flagged
	w = doJSON(t, r, http.MethodPost, "/api/v1/orders",
		map[string]any{"items": []map[string]any{{"product_id": p1.ID, "quantity": 2}}}, ctok)
	if w.Code != http.StatusCreated {
		t.Fatalf("drain order: %d", w.Code)
	}
	w = doJSON(t, r, http.MethodGet, "/api/v1/products/"+itoa(p1.ID), nil, "")
	decode(t, w, &got)
	if got.Stock != 0 || !got.LowStock {
		t.Fatalf("after drain: want stock=0 low_stock=true, got stock=%d low_stock=%v", got.Stock, got.LowStock)
	}

	// can't order from empty stock
	w = doJSON(t, r, http.MethodPost, "/api/v1/orders",
		map[string]any{"items": []map[string]any{{"product_id": p1.ID, "quantity": 1}}}, ctok)
	if w.Code != http.StatusConflict {
		t.Fatalf("post-drain order: want 409, got %d", w.Code)
	}
}
