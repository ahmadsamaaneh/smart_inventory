package handlers

import (
	"strconv"
	"strings"

	"github.com/ahmad/smart-inventory/internal/httpx"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ProductHandler struct {
	DB *gorm.DB
}

type productCreateReq struct {
	SKU         string  `json:"sku" binding:"required,min=1,max=64"`
	Name        string  `json:"name" binding:"required,min=1,max=255"`
	Description string  `json:"description"`
	Category    string  `json:"category"`
	Material    string  `json:"material" binding:"required,min=1,max=128"`
	Size        string  `json:"size" binding:"required,min=1,max=64"`
	ThicknessMM float64 `json:"thickness_mm" binding:"required,gt=0"`
	PriceCents  int64   `json:"price_cents" binding:"required,min=0"`
	Stock       int     `json:"stock" binding:"min=0"`
	LowStockAt  int     `json:"low_stock_at" binding:"min=0"`
	Active      *bool   `json:"active"`
}

type productUpdateReq struct {
	Name        *string  `json:"name" binding:"omitempty,min=1,max=255"`
	Description *string  `json:"description"`
	Category    *string  `json:"category"`
	Material    *string  `json:"material" binding:"omitempty,min=1,max=128"`
	Size        *string  `json:"size" binding:"omitempty,min=1,max=64"`
	ThicknessMM *float64 `json:"thickness_mm" binding:"omitempty,gt=0"`
	PriceCents  *int64   `json:"price_cents" binding:"omitempty,min=0"`
	LowStockAt  *int     `json:"low_stock_at" binding:"omitempty,min=0"`
	Active      *bool    `json:"active"`
}

type listResp struct {
	Items []models.Product `json:"items"`
	Total int64            `json:"total"`
	Page  int              `json:"page"`
	Size  int              `json:"size"`
}

// List returns a paginated, filterable list of products.
func (h *ProductHandler) List(c *gin.Context) {
	q := h.DB.Model(&models.Product{})

	if s := strings.TrimSpace(c.Query("q")); s != "" {
		like := "%" + strings.ToLower(s) + "%"
		q = q.Where("LOWER(name) LIKE ? OR LOWER(sku) LIKE ?", like, like)
	}
	if cat := strings.TrimSpace(c.Query("category")); cat != "" {
		q = q.Where("category = ?", cat)
	}
	if mat := strings.TrimSpace(c.Query("material")); mat != "" {
		q = q.Where("material = ?", mat)
	}
	if a := c.Query("active"); a != "" {
		q = q.Where("active = ?", a == "true" || a == "1")
	}

	page, _ := strconv.Atoi(c.DefaultQuery("page", "1"))
	if page < 1 {
		page = 1
	}
	size, _ := strconv.Atoi(c.DefaultQuery("size", "20"))
	if size < 1 || size > 100 {
		size = 20
	}

	var total int64
	if err := q.Count(&total).Error; err != nil {
		httpx.Internal(c, err)
		return
	}
	var items []models.Product
	if err := q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error; err != nil {
		httpx.Internal(c, err)
		return
	}
	httpx.OK(c, listResp{Items: items, Total: total, Page: page, Size: size})
}

func (h *ProductHandler) Get(c *gin.Context) {
	var p models.Product
	if err := h.DB.First(&p, c.Param("id")).Error; err != nil {
		httpx.HandleDBError(c, err)
		return
	}
	httpx.OK(c, p)
}

func (h *ProductHandler) Create(c *gin.Context) {
	var req productCreateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}
	active := true
	if req.Active != nil {
		active = *req.Active
	}
	low := req.LowStockAt
	if low == 0 {
		low = 5 // sensible default
	}
	p := models.Product{
		SKU:         strings.TrimSpace(req.SKU),
		Name:        req.Name,
		Description: req.Description,
		Category:    req.Category,
		Material:    req.Material,
		Size:        req.Size,
		ThicknessMM: req.ThicknessMM,
		PriceCents:  req.PriceCents,
		Stock:       req.Stock,
		LowStockAt:  low,
		Active:      active,
	}
	if err := h.DB.Create(&p).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			httpx.Conflict(c, "SKU already exists")
			return
		}
		httpx.Internal(c, err)
		return
	}
	if p.Stock != 0 {
		_ = h.DB.Create(&models.StockMovement{
			ProductID: p.ID, Delta: p.Stock, Reason: "initial",
		}).Error
	}
	httpx.Created(c, p)
}

func (h *ProductHandler) Update(c *gin.Context) {
	var req productUpdateReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}
	var p models.Product
	if err := h.DB.First(&p, c.Param("id")).Error; err != nil {
		httpx.HandleDBError(c, err)
		return
	}
	updates := map[string]any{}
	if req.Name != nil {
		updates["name"] = *req.Name
	}
	if req.Description != nil {
		updates["description"] = *req.Description
	}
	if req.Category != nil {
		updates["category"] = *req.Category
	}
	if req.Material != nil {
		updates["material"] = *req.Material
	}
	if req.Size != nil {
		updates["size"] = *req.Size
	}
	if req.ThicknessMM != nil {
		updates["thickness_mm"] = *req.ThicknessMM
	}
	if req.PriceCents != nil {
		updates["price_cents"] = *req.PriceCents
	}
	if req.LowStockAt != nil {
		updates["low_stock_at"] = *req.LowStockAt
	}
	if req.Active != nil {
		updates["active"] = *req.Active
	}
	if len(updates) == 0 {
		httpx.BadRequest(c, "no fields to update", nil)
		return
	}
	if err := h.DB.Model(&p).Updates(updates).Error; err != nil {
		httpx.Internal(c, err)
		return
	}
	httpx.OK(c, p)
}

func (h *ProductHandler) Delete(c *gin.Context) {
	if err := h.DB.Delete(&models.Product{}, c.Param("id")).Error; err != nil {
		httpx.Internal(c, err)
		return
	}
	c.Status(204)
}
