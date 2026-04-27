package server

import (
	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/handlers"
	"github.com/ahmad/smart-inventory/internal/middleware"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

func NewRouter(db *gorm.DB, jwtMgr *auth.Manager) *gin.Engine {
	gin.SetMode(gin.ReleaseMode)
	r := gin.New()
	r.Use(middleware.Logger(), middleware.Recovery(), cors.Default())

	r.GET("/health", func(c *gin.Context) {
		c.JSON(200, gin.H{"status": "ok"})
	})

	authH := &handlers.AuthHandler{DB: db, JWTMgr: jwtMgr}
	prodH := &handlers.ProductHandler{DB: db}
	invH := &handlers.InventoryHandler{DB: db}
	ordH := &handlers.OrderHandler{DB: db}

	v1 := r.Group("/api/v1")
	{
		a := v1.Group("/auth")
		a.POST("/register", authH.Register)
		a.POST("/login", authH.Login)
		a.GET("/me", middleware.Auth(jwtMgr), authH.Me)

		v1.GET("/products", prodH.List)
		v1.GET("/products/:id", prodH.Get)

		// need a valid token for everything below
		authed := v1.Group("")
		authed.Use(middleware.Auth(jwtMgr))
		{
			authed.POST("/orders", ordH.Create)
			authed.GET("/orders", ordH.List)
			authed.GET("/orders/:id", ordH.Get)
			authed.POST("/orders/:id/cancel", ordH.Cancel)

			authed.GET("/inventory/:product_id", invH.GetByProduct)
		}

		// admin-only routes
		admin := v1.Group("")
		admin.Use(middleware.Auth(jwtMgr), middleware.RequireRole(models.RoleAdmin))
		{
			admin.POST("/products", prodH.Create)
			admin.PUT("/products/:id", prodH.Update)
			admin.DELETE("/products/:id", prodH.Delete)

			admin.POST("/inventory/add", invH.AddStock)
			admin.POST("/inventory/remove", invH.RemoveStock)
			admin.POST("/products/:id/stock", invH.AdjustStock) // legacy delta endpoint
			admin.GET("/inventory/low-stock", invH.LowStock)
			admin.GET("/inventory/movements", invH.Movements)

			admin.POST("/orders/:id/status", ordH.UpdateStatus)
		}
	}
	return r
}
