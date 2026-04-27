package handlers_test

import (
	"net/http"
	"testing"

	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/models"
)

func TestOrders_MultiProductCreateAndTotal(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	// Seed a customer and two products.
	hash, _ := auth.HashPassword("Passw0rd!")
	cust := &models.User{Email: "buyer@x.com", PasswordHash: hash, Name: "Buyer", Role: models.RoleCustomer}
	if err := db.Create(cust).Error; err != nil {
		t.Fatalf("seed user: %v", err)
	}
	tok, _, _ := jwtMgr.Issue(cust)

	pA := &models.Product{
		SKU: "P-A", Name: "Tile A", Material: "marble", Size: "60x60",
		ThicknessMM: 10, PriceCents: 2500, Stock: 10, LowStockAt: 1, Active: true,
	}
	pB := &models.Product{
		SKU: "P-B", Name: "Tile B", Material: "granite", Size: "30x60",
		ThicknessMM: 12, PriceCents: 1700, Stock: 5, LowStockAt: 1, Active: true,
	}
	if err := db.Create(pA).Error; err != nil {
		t.Fatalf("seed pA: %v", err)
	}
	if err := db.Create(pB).Error; err != nil {
		t.Fatalf("seed pB: %v", err)
	}

	// 3xA + 2xB = 7500 + 3400 = 10900
	body := map[string]any{
		"items": []map[string]any{
			{"product_id": pA.ID, "quantity": 3},
			{"product_id": pB.ID, "quantity": 2},
		},
	}
	w := doJSON(t, r, http.MethodPost, "/api/v1/orders", body, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("create order: want 201, got %d body=%s", w.Code, w.Body.String())
	}

	var got models.Order
	decode(t, w, &got)
	if got.ID == 0 {
		t.Fatalf("order id is zero: %+v", got)
	}
	if got.TotalCents != 10900 {
		t.Fatalf("total mismatch: want 10900, got %d", got.TotalCents)
	}
	if len(got.Items) != 2 {
		t.Fatalf("expected 2 items, got %d", len(got.Items))
	}
	for _, it := range got.Items {
		switch it.ProductID {
		case pA.ID:
			if it.Quantity != 3 || it.UnitPriceCents != 2500 {
				t.Fatalf("A item wrong: %+v", it)
			}
		case pB.ID:
			if it.Quantity != 2 || it.UnitPriceCents != 1700 {
				t.Fatalf("B item wrong: %+v", it)
			}
		default:
			t.Fatalf("unexpected product in item: %+v", it)
		}
	}

	// refetch via API to make sure it persisted
	w = doJSON(t, r, http.MethodGet, "/api/v1/orders/"+itoa(got.ID), nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("get order: want 200, got %d", w.Code)
	}
	var fetched models.Order
	decode(t, w, &fetched)
	if fetched.TotalCents != 10900 || len(fetched.Items) != 2 {
		t.Fatalf("get returned wrong order: %+v", fetched)
	}

	// also check DB directly
	var dbOrder models.Order
	if err := db.Preload("Items").First(&dbOrder, got.ID).Error; err != nil {
		t.Fatalf("DB refetch: %v", err)
	}
	if dbOrder.TotalCents != 10900 || len(dbOrder.Items) != 2 {
		t.Fatalf("DB order mismatch: %+v", dbOrder)
	}

	// stock should be deducted: A 10->7, B 5->3
	var freshA, freshB models.Product
	_ = db.First(&freshA, pA.ID).Error
	_ = db.First(&freshB, pB.ID).Error
	if freshA.Stock != 7 || freshB.Stock != 3 {
		t.Fatalf("stock not deducted correctly: A=%d B=%d", freshA.Stock, freshB.Stock)
	}

	// list endpoint should show our order
	w = doJSON(t, r, http.MethodGet, "/api/v1/orders", nil, tok)
	if w.Code != http.StatusOK {
		t.Fatalf("list: want 200, got %d", w.Code)
	}
	var list struct {
		Items []models.Order `json:"items"`
		Total int64          `json:"total"`
	}
	decode(t, w, &list)
	if list.Total < 1 || len(list.Items) < 1 {
		t.Fatalf("list returned no orders: %+v", list)
	}
}

// If the client sends the same product_id twice, we merge them into one row.
func TestOrders_DuplicateProductIDsCombined(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	hash, _ := auth.HashPassword("x")
	cust := &models.User{Email: "d@x.com", PasswordHash: hash, Name: "D", Role: models.RoleCustomer}
	_ = db.Create(cust).Error
	tok, _, _ := jwtMgr.Issue(cust)

	p := &models.Product{
		SKU: "P-D", Name: "Slab", Material: "marble", Size: "60x60",
		ThicknessMM: 10, PriceCents: 1000, Stock: 10, LowStockAt: 1, Active: true,
	}
	_ = db.Create(p).Error

	body := map[string]any{
		"items": []map[string]any{
			{"product_id": p.ID, "quantity": 2},
			{"product_id": p.ID, "quantity": 3}, // same product
		},
	}
	w := doJSON(t, r, http.MethodPost, "/api/v1/orders", body, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: want 201, got %d body=%s", w.Code, w.Body.String())
	}
	var o models.Order
	decode(t, w, &o)
	if o.TotalCents != 5000 {
		t.Fatalf("total: want 5000 (5*1000), got %d", o.TotalCents)
	}
	if len(o.Items) != 1 || o.Items[0].Quantity != 5 {
		t.Fatalf("expected one combined item qty=5, got %+v", o.Items)
	}
}

// Price snapshot: changing the product price after ordering shouldn't
// affect historical orders.
func TestOrders_PriceSnapshotIsImmutable(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	hash, _ := auth.HashPassword("x")
	cust := &models.User{Email: "s@x.com", PasswordHash: hash, Name: "S", Role: models.RoleCustomer}
	_ = db.Create(cust).Error
	tok, _, _ := jwtMgr.Issue(cust)

	p := &models.Product{
		SKU: "P-S", Name: "Slab", Material: "marble", Size: "60x60",
		ThicknessMM: 10, PriceCents: 1000, Stock: 10, LowStockAt: 1, Active: true,
	}
	_ = db.Create(p).Error

	body := map[string]any{
		"items": []map[string]any{{"product_id": p.ID, "quantity": 2}},
	}
	w := doJSON(t, r, http.MethodPost, "/api/v1/orders", body, tok)
	if w.Code != http.StatusCreated {
		t.Fatalf("create: %d %s", w.Code, w.Body.String())
	}
	var o models.Order
	decode(t, w, &o)

	// bump the price after the order
	if err := db.Model(&models.Product{}).Where("id = ?", p.ID).Update("price_cents", 9999).Error; err != nil {
		t.Fatalf("price bump: %v", err)
	}

	w = doJSON(t, r, http.MethodGet, "/api/v1/orders/"+itoa(o.ID), nil, tok)
	var after models.Order
	decode(t, w, &after)
	if after.TotalCents != 2000 {
		t.Fatalf("historical total mutated: %d", after.TotalCents)
	}
	if len(after.Items) != 1 || after.Items[0].UnitPriceCents != 1000 {
		t.Fatalf("snapshot price mutated: %+v", after.Items)
	}
}

func TestOrders_InsufficientStockReturns409(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	hash, _ := auth.HashPassword("x")
	cust := &models.User{Email: "i@x.com", PasswordHash: hash, Name: "I", Role: models.RoleCustomer}
	_ = db.Create(cust).Error
	tok, _, _ := jwtMgr.Issue(cust)

	p := &models.Product{
		SKU: "P-I", Name: "Tile", Material: "marble", Size: "60x60",
		ThicknessMM: 10, PriceCents: 100, Stock: 2, LowStockAt: 1, Active: true,
	}
	_ = db.Create(p).Error

	body := map[string]any{"items": []map[string]any{{"product_id": p.ID, "quantity": 5}}}
	w := doJSON(t, r, http.MethodPost, "/api/v1/orders", body, tok)
	if w.Code != http.StatusConflict {
		t.Fatalf("want 409, got %d", w.Code)
	}
	// Stock unchanged
	var fresh models.Product
	_ = db.First(&fresh, p.ID).Error
	if fresh.Stock != 2 {
		t.Fatalf("stock changed despite rejection: %d", fresh.Stock)
	}
	// No order persisted
	var count int64
	_ = db.Model(&models.Order{}).Count(&count).Error
	if count != 0 {
		t.Fatalf("expected 0 orders, got %d", count)
	}
}

func TestOrders_RBAC_CustomerCannotSeeOthers(t *testing.T) {
	r, db, jwtMgr := setupTestServer(t)

	hash, _ := auth.HashPassword("x")
	a := &models.User{Email: "a@x.com", PasswordHash: hash, Name: "A", Role: models.RoleCustomer}
	b := &models.User{Email: "b@x.com", PasswordHash: hash, Name: "B", Role: models.RoleCustomer}
	_ = db.Create(a).Error
	_ = db.Create(b).Error
	atok, _, _ := jwtMgr.Issue(a)
	btok, _, _ := jwtMgr.Issue(b)

	p := &models.Product{
		SKU: "P-R", Name: "Tile", Material: "marble", Size: "60x60",
		ThicknessMM: 10, PriceCents: 100, Stock: 5, LowStockAt: 1, Active: true,
	}
	_ = db.Create(p).Error

	w := doJSON(t, r, http.MethodPost, "/api/v1/orders",
		map[string]any{"items": []map[string]any{{"product_id": p.ID, "quantity": 1}}}, atok)
	if w.Code != http.StatusCreated {
		t.Fatalf("a create: %d", w.Code)
	}
	var oa models.Order
	decode(t, w, &oa)

	// B shouldn't be able to read A's order
	w = doJSON(t, r, http.MethodGet, "/api/v1/orders/"+itoa(oa.ID), nil, btok)
	if w.Code != http.StatusForbidden {
		t.Fatalf("cross-user get: want 403, got %d", w.Code)
	}

	// B's order list should be empty
	w = doJSON(t, r, http.MethodGet, "/api/v1/orders", nil, btok)
	var list struct {
		Items []models.Order `json:"items"`
		Total int64          `json:"total"`
	}
	decode(t, w, &list)
	if list.Total != 0 || len(list.Items) != 0 {
		t.Fatalf("B should have no orders: %+v", list)
	}
}
