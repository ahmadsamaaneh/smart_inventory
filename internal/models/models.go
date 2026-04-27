package models

import (
	"time"

	"gorm.io/gorm"
)

// Role controls what a user can do (admin vs regular customer).
type Role string

const (
	RoleAdmin    Role = "admin"
	RoleCustomer Role = "customer"
)

type OrderStatus string

const (
	OrderPending   OrderStatus = "pending"
	OrderPaid      OrderStatus = "paid"
	OrderShipped   OrderStatus = "shipped"
	OrderDelivered OrderStatus = "delivered"
	OrderCancelled OrderStatus = "cancelled"
	// TODO: maybe add "refunded" later
)

type User struct {
	ID           uint           `gorm:"primaryKey" json:"id"`
	Email        string         `gorm:"uniqueIndex;size:255;not null" json:"email"`
	PasswordHash string         `gorm:"not null" json:"-"`
	Name         string         `gorm:"size:255" json:"name"`
	Role         Role           `gorm:"size:32;not null;default:customer" json:"role"`
	CreatedAt    time.Time      `json:"created_at"`
	UpdatedAt    time.Time      `json:"updated_at"`
	DeletedAt    gorm.DeletedAt `gorm:"index" json:"-"`
}

type Product struct {
	ID          uint   `gorm:"primaryKey" json:"id"`
	SKU         string `gorm:"uniqueIndex;size:64;not null" json:"sku"`
	Name        string `gorm:"size:255;not null" json:"name"`
	Description string `gorm:"type:text" json:"description"`
	Category    string `gorm:"size:128;index" json:"category"`

	Material    string  `gorm:"size:128;not null;index" json:"material"`
	Size        string  `gorm:"size:64;not null" json:"size"`         // e.g. "60x60", "30x60"
	ThicknessMM float64 `gorm:"not null" json:"thickness_mm"`

	PriceCents int64 `gorm:"not null" json:"price_cents"`
	Stock      int   `gorm:"not null;default:0" json:"stock"`
	LowStockAt int   `gorm:"not null;default:5" json:"low_stock_at"`
	Active     bool  `gorm:"not null;default:true" json:"active"`

	LowStock bool `gorm:"-" json:"low_stock"` // not stored, computed on read

	CreatedAt time.Time      `json:"created_at"`
	UpdatedAt time.Time      `json:"updated_at"`
	DeletedAt gorm.DeletedAt `gorm:"index" json:"-"`
}

func (p *Product) AfterFind(_ *gorm.DB) error {
	p.LowStock = p.Stock <= p.LowStockAt
	return nil
}

type Order struct {
	ID         uint           `gorm:"primaryKey" json:"id"`
	UserID     uint           `gorm:"index;not null" json:"user_id"`
	User       *User          `json:"user,omitempty"`
	Status     OrderStatus    `gorm:"size:32;not null;default:pending;index" json:"status"`
	TotalCents int64          `gorm:"not null;default:0" json:"total_cents"`
	Items      []OrderItem    `gorm:"constraint:OnDelete:CASCADE" json:"items,omitempty"`
	CreatedAt  time.Time      `json:"created_at"`
	UpdatedAt  time.Time      `json:"updated_at"`
	DeletedAt  gorm.DeletedAt `gorm:"index" json:"-"`
}

type OrderItem struct {
	ID             uint     `gorm:"primaryKey" json:"id"`
	OrderID        uint     `gorm:"index;not null" json:"order_id"`
	ProductID      uint     `gorm:"index;not null" json:"product_id"`
	Product        *Product `json:"product,omitempty"`
	Quantity       int      `gorm:"not null" json:"quantity"`
	UnitPriceCents int64    `gorm:"not null" json:"unit_price_cents"`
}

// StockMovement tracks every stock change for auditing.
type StockMovement struct {
	ID        uint      `gorm:"primaryKey" json:"id"`
	ProductID uint      `gorm:"index;not null" json:"product_id"`
	Delta     int       `gorm:"not null" json:"delta"`
	Reason    string    `gorm:"size:128" json:"reason"`
	RefType   string    `gorm:"size:64" json:"ref_type,omitempty"`
	RefID     uint      `json:"ref_id,omitempty"`
	CreatedAt time.Time `json:"created_at"`
}
