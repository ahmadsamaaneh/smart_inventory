package handlers

import (
	"errors"
	"fmt"
	"strconv"

	"github.com/ahmad/smart-inventory/internal/httpx"
	"github.com/ahmad/smart-inventory/internal/middleware"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
	"gorm.io/gorm/clause"
)

type OrderHandler struct {
	DB *gorm.DB
}

type orderItemReq struct {
	ProductID uint `json:"product_id" binding:"required"`
	Quantity  int  `json:"quantity" binding:"required,min=1"`
}

type createOrderReq struct {
	Items []orderItemReq `json:"items" binding:"required,min=1,dive"`
}

type updateStatusReq struct {
	Status models.OrderStatus `json:"status" binding:"required"`
}

var validStatuses = map[models.OrderStatus]bool{
	models.OrderPending:   true,
	models.OrderPaid:      true,
	models.OrderShipped:   true,
	models.OrderDelivered: true,
	models.OrderCancelled: true,
}

// Create handles new order placement. Validates stock, deducts it
// inside a tx, and snapshots the unit prices at time of purchase.
func (h *OrderHandler) Create(c *gin.Context) {
	var req createOrderReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}

	// merge dupes (in case the client sends the same product twice)
	qty := make(map[uint]int, len(req.Items))
	for _, it := range req.Items {
		qty[it.ProductID] += it.Quantity
	}

	uid := middleware.UserID(c)
	var created models.Order

	err := h.DB.Transaction(func(tx *gorm.DB) error {
		order := models.Order{UserID: uid, Status: models.OrderPending}
		if err := tx.Create(&order).Error; err != nil {
			return err
		}

		var total int64
		useLock := tx.Dialector.Name() == "postgres"
		for pid, q := range qty {
			var p models.Product
			q2 := tx
			if useLock {
				q2 = tx.Clauses(clause.Locking{Strength: "UPDATE"})
			}
			if err := q2.First(&p, pid).Error; err != nil {
				if errors.Is(err, gorm.ErrRecordNotFound) {
					return fmt.Errorf("product %d: %w", pid, gorm.ErrRecordNotFound)
				}
				return err
			}
			if !p.Active {
				return fmt.Errorf("product %d is not active", pid)
			}
			if p.Stock < q {
				return fmt.Errorf("product %d: %w (have %d, need %d)", pid, errInsufficientStock, p.Stock, q)
			}
			if err := tx.Model(&p).Update("stock", p.Stock-q).Error; err != nil {
				return err
			}
			item := models.OrderItem{
				OrderID:        order.ID,
				ProductID:      p.ID,
				Quantity:       q,
				UnitPriceCents: p.PriceCents,
			}
			if err := tx.Create(&item).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.StockMovement{
				ProductID: p.ID, Delta: -q, Reason: "order", RefType: "order", RefID: order.ID,
			}).Error; err != nil {
				return err
			}
			total += p.PriceCents * int64(q)
		}
		if err := tx.Model(&order).Update("total_cents", total).Error; err != nil {
			return err
		}
		created = order
		created.TotalCents = total
		return tx.Preload("Items").First(&created, order.ID).Error
	})

	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			httpx.NotFound(c, err.Error())
		case errors.Is(err, errInsufficientStock):
			httpx.Conflict(c, err.Error())
		default:
			httpx.BadRequest(c, err.Error(), nil)
		}
		return
	}
	httpx.Created(c, created)
}

func (h *OrderHandler) List(c *gin.Context) {
	q := h.DB.Model(&models.Order{}).Preload("Items")
	if middleware.UserRole(c) != models.RoleAdmin {
		q = q.Where("user_id = ?", middleware.UserID(c))
	} else if uid := c.Query("user_id"); uid != "" {
		q = q.Where("user_id = ?", uid)
	}
	if s := c.Query("status"); s != "" {
		q = q.Where("status = ?", s)
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
	var items []models.Order
	if err := q.Order("id DESC").Limit(size).Offset((page - 1) * size).Find(&items).Error; err != nil {
		httpx.Internal(c, err)
		return
	}
	httpx.OK(c, gin.H{"items": items, "total": total, "page": page, "size": size})
}

func (h *OrderHandler) Get(c *gin.Context) {
	var o models.Order
	if err := h.DB.Preload("Items.Product").First(&o, c.Param("id")).Error; err != nil {
		httpx.HandleDBError(c, err)
		return
	}
	if middleware.UserRole(c) != models.RoleAdmin && o.UserID != middleware.UserID(c) {
		httpx.Forbidden(c, "")
		return
	}
	httpx.OK(c, o)
}

// Cancel an order and restore the stock. Customers can only cancel
// their own pending/paid orders; admins can cancel anything.
func (h *OrderHandler) Cancel(c *gin.Context) {
	id := c.Param("id")
	err := h.DB.Transaction(func(tx *gorm.DB) error {
		var o models.Order
		if err := tx.Preload("Items").First(&o, id).Error; err != nil {
			return err
		}
		isAdmin := middleware.UserRole(c) == models.RoleAdmin
		if !isAdmin && o.UserID != middleware.UserID(c) {
			return errForbidden
		}
		if o.Status == models.OrderCancelled {
			return errors.New("order already cancelled")
		}
		if o.Status == models.OrderShipped || o.Status == models.OrderDelivered {
			if !isAdmin {
				return errors.New("cannot cancel shipped or delivered order")
			}
		}
		for _, it := range o.Items {
			if err := tx.Model(&models.Product{}).Where("id = ?", it.ProductID).
				UpdateColumn("stock", gorm.Expr("stock + ?", it.Quantity)).Error; err != nil {
				return err
			}
			if err := tx.Create(&models.StockMovement{
				ProductID: it.ProductID, Delta: it.Quantity, Reason: "order_cancel", RefType: "order", RefID: o.ID,
			}).Error; err != nil {
				return err
			}
		}
		return tx.Model(&o).Update("status", models.OrderCancelled).Error
	})
	if err != nil {
		switch {
		case errors.Is(err, gorm.ErrRecordNotFound):
			httpx.NotFound(c, "order not found")
		case errors.Is(err, errForbidden):
			httpx.Forbidden(c, "")
		default:
			httpx.BadRequest(c, err.Error(), nil)
		}
		return
	}
	var o models.Order
	_ = h.DB.Preload("Items").First(&o, id).Error
	httpx.OK(c, o)
}

func (h *OrderHandler) UpdateStatus(c *gin.Context) {
	var req updateStatusReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}
	if !validStatuses[req.Status] {
		httpx.BadRequest(c, "invalid status", nil)
		return
	}
	var o models.Order
	if err := h.DB.First(&o, c.Param("id")).Error; err != nil {
		httpx.HandleDBError(c, err)
		return
	}
	// don't allow cancel through here — use the cancel endpoint so stock gets restored
	if req.Status == models.OrderCancelled {
		httpx.BadRequest(c, "use cancel endpoint to cancel an order", nil)
		return
	}
	if err := h.DB.Model(&o).Update("status", req.Status).Error; err != nil {
		httpx.Internal(c, err)
		return
	}
	o.Status = req.Status
	httpx.OK(c, o)
}

var errForbidden = errors.New("forbidden")
