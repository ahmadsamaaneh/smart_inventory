package middleware

import (
	"log/slog"
	"strings"
	"time"

	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/httpx"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/gin-gonic/gin"
)

const (
	ctxUserID = "user_id"
	ctxRole   = "user_role"
	ctxEmail  = "user_email"
)

func Logger() gin.HandlerFunc {
	return func(c *gin.Context) {
		start := time.Now()
		c.Next()
		slog.Info("http",
			"method", c.Request.Method,
			"path", c.Request.URL.Path,
			"status", c.Writer.Status(),
			"dur_ms", time.Since(start).Milliseconds(),
			"ip", c.ClientIP(),
		)
	}
}

func Recovery() gin.HandlerFunc {
	return gin.CustomRecovery(func(c *gin.Context, recovered any) {
		slog.Error("panic recovered", "err", recovered, "path", c.Request.URL.Path)
		c.AbortWithStatusJSON(500, gin.H{"error": "internal error"})
	})
}

func Auth(jwtMgr *auth.Manager) gin.HandlerFunc {
	return func(c *gin.Context) {
		h := c.GetHeader("Authorization")
		if h == "" || !strings.HasPrefix(strings.ToLower(h), "bearer ") {
			httpx.Unauthorized(c, "missing bearer token")
			return
		}
		tok := strings.TrimSpace(h[7:])
		claims, err := jwtMgr.Parse(tok)
		if err != nil {
			httpx.Unauthorized(c, "invalid token")
			return
		}
		c.Set(ctxUserID, claims.UserID)
		c.Set(ctxRole, string(claims.Role))
		c.Set(ctxEmail, claims.Email)
		c.Next()
	}
}

// RequireRole rejects requests from users not in the given role list.
func RequireRole(roles ...models.Role) gin.HandlerFunc {
	allowed := make(map[string]struct{}, len(roles))
	for _, r := range roles {
		allowed[string(r)] = struct{}{}
	}
	return func(c *gin.Context) {
		role, _ := c.Get(ctxRole)
		rs, _ := role.(string)
		if _, ok := allowed[rs]; !ok {
			httpx.Forbidden(c, "insufficient role")
			return
		}
		c.Next()
	}
}

func UserID(c *gin.Context) uint {
	v, _ := c.Get(ctxUserID)
	id, _ := v.(uint)
	return id
}

func UserRole(c *gin.Context) models.Role {
	v, _ := c.Get(ctxRole)
	s, _ := v.(string)
	return models.Role(s)
}
