package handlers

import (
	"errors"

	"github.com/ahmad/smart-inventory/internal/httpx"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type InventoryHandler struct {
	DB *gorm.DB
}

type adjustStockReq struct {
	Delta  int    `json:"delta" binding:"required"`
	Reason string `json:"reason" binding:"required,min=1,max=128"`
}

type stockChangeReq struct {
	ProductID uint   `json:"product_id" binding:"required"`
	Quantity  int    `json:"quantity"  binding:"required,gt=0"`
	Reason    string `json:"reason"    binding:"omitempty,max=128"`
}

type inventoryView struct {
	ProductID  uint   `json:"product_id"`
	SKU        string `json:"sku"`
	Name       string `json:"name"`
	Quantity   int    `json:"quantity"`
	LowStockAt int    `json:"low_stock_at"`
	LowStock   bool   `json:"low_stock"`
}

func toInventoryView(p models.Product) inventoryView {
	return inventoryView{
		ProductID:  p.ID,
		SKU:        p.SKU,
		Name:       p.Name,
		Quantity:   p.Stock,
		LowStockAt: p.LowStockAt,
		LowStock:   p.Stock <= p.LowStockAt,
	}
}

// applyDelta handles all stock changes in a single transaction.
// Rejects anything that would make stock go negative.
func (h *InventoryHandler) applyDelta(productID uint, delta int, reason, refType string, refID uint) (models.Product, error) {
	var out models.Product
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var p models.Product
		if err := tx.First(&p, productID).Error; err != nil {
			return err
		}
		newStock := p.Stock + delta
		if newStock < 0 {
			return errInsufficientStock
		}
		if err := tx.Model(&p).Update("stock", newStock).Error; err != nil {
			return err
		}
		if err := tx.Create(&models.StockMovement{
			ProductID: p.ID, Delta: delta, Reason: reason, RefType: refType, RefID: refID,
		}).Error; err != nil {
			return err
		}
		p.Stock = newStock
		out = p
		return nil
	})
	return out, err
}

// AdjustStock — signed delta endpoint (kept around for backwards compat).
func (h *InventoryHandler) AdjustStock(c *gin.Context) {
	var req adjustStockReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}
	pid, ok := parseUintParam(c, "id")
	if !ok {
		return
	}
	p, err := h.applyDelta(pid, req.Delta, req.Reason, "manual", 0)
	if mapAdjustErr(c, err) {
		return
	}
	httpx.OK(c, toInventoryView(p))
}

// AddStock bumps up the stock count for a product.
func (h *InventoryHandler) AddStock(c *gin.Context) {
	var req stockChangeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}
	reason := req.Reason
	if reason == "" {
		reason = "restock"
	}
	p, err := h.applyDelta(req.ProductID, req.Quantity, reason, "manual", 0)
	if mapAdjustErr(c, err) {
		return
	}
	httpx.OK(c, toInventoryView(p))
}

// RemoveStock pulls stock out. Returns 409 if it would go negative.
func (h *InventoryHandler) RemoveStock(c *gin.Context) {
	var req stockChangeReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}
	reason := req.Reason
	if reason == "" {
		reason = "manual_decrement"
	}
	p, err := h.applyDelta(req.ProductID, -req.Quantity, reason, "manual", 0)
	if mapAdjustErr(c, err) {
		return
	}
	httpx.OK(c, toInventoryView(p))
}

// GetByProduct returns current stock info for a single product.
func (h *InventoryHandler) GetByProduct(c *gin.Context) {
	pid, ok := parseUintParam(c, "product_id")
	if !ok {
		return
	}
	var p models.Product
	if err := h.DB.First(&p, pid).Error; err != nil {
		httpx.HandleDBError(c, err)
		return
	}
	httpx.OK(c, toInventoryView(p))
}

func mapAdjustErr(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	switch {
	case errors.Is(err, gorm.ErrRecordNotFound):
		httpx.NotFound(c, "product not found")
	case errors.Is(err, errInsufficientStock):
		httpx.Conflict(c, "insufficient stock")
	default:
		httpx.Internal(c, err)
	}
	return true
}

func (h *InventoryHandler) LowStock(c *gin.Context) {
	var items []models.Product
	if err := h.DB.Where("stock <= low_stock_at AND active = ?", true).Order("stock ASC").Find(&items).Error; err != nil {
		httpx.Internal(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items, "count": len(items)})
}

func (h *InventoryHandler) Movements(c *gin.Context) {
	var items []models.StockMovement
	q := h.DB.Model(&models.StockMovement{})
	if pid := c.Query("product_id"); pid != "" {
		q = q.Where("product_id = ?", pid)
	}
	if err := q.Order("id DESC").Limit(200).Find(&items).Error; err != nil {
		httpx.Internal(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items})
}

var errInsufficientStock = errors.New("insufficient stock")
