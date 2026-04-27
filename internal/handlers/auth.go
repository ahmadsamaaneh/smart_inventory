package handlers

import (
	"errors"
	"strings"

	"github.com/ahmad/smart-inventory/internal/auth"
	"github.com/ahmad/smart-inventory/internal/httpx"
	"github.com/ahmad/smart-inventory/internal/middleware"
	"github.com/ahmad/smart-inventory/internal/models"
	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type AuthHandler struct {
	DB     *gorm.DB
	JWTMgr *auth.Manager
}

type registerReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required,min=8,max=128"`
	Name     string `json:"name" binding:"required,min=1,max=255"`
}

type loginReq struct {
	Email    string `json:"email" binding:"required,email"`
	Password string `json:"password" binding:"required"`
}

type tokenResp struct {
	Token     string       `json:"token"`
	ExpiresAt string       `json:"expires_at"`
	User      *models.User `json:"user"`
}

func (h *AuthHandler) Register(c *gin.Context) {
	var req registerReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}
	hash, err := auth.HashPassword(req.Password)
	if err != nil {
		httpx.Internal(c, err)
		return
	}
	u := &models.User{
		Email:        strings.ToLower(strings.TrimSpace(req.Email)),
		PasswordHash: hash,
		Name:         req.Name,
		Role:         models.RoleCustomer,
	}
	if err := h.DB.Create(u).Error; err != nil {
		if strings.Contains(strings.ToLower(err.Error()), "unique") {
			httpx.Conflict(c, "email already registered")
			return
		}
		httpx.Internal(c, err)
		return
	}
	tok, exp, err := h.JWTMgr.Issue(u)
	if err != nil {
		httpx.Internal(c, err)
		return
	}
	httpx.Created(c, tokenResp{Token: tok, ExpiresAt: exp.Format("2006-01-02T15:04:05Z07:00"), User: u})
}

func (h *AuthHandler) Login(c *gin.Context) {
	var req loginReq
	if err := c.ShouldBindJSON(&req); err != nil {
		httpx.BadRequest(c, "invalid body", err.Error())
		return
	}
	var u models.User
	if err := h.DB.Where("email = ?", strings.ToLower(strings.TrimSpace(req.Email))).First(&u).Error; err != nil {
		if errors.Is(err, gorm.ErrRecordNotFound) {
			httpx.Unauthorized(c, "invalid credentials")
			return
		}
		httpx.Internal(c, err)
		return
	}
	if !auth.CheckPassword(u.PasswordHash, req.Password) {
		httpx.Unauthorized(c, "invalid credentials")
		return
	}
	tok, exp, err := h.JWTMgr.Issue(&u)
	if err != nil {
		httpx.Internal(c, err)
		return
	}
	httpx.OK(c, tokenResp{Token: tok, ExpiresAt: exp.Format("2006-01-02T15:04:05Z07:00"), User: &u})
}

func (h *AuthHandler) Me(c *gin.Context) {
	var u models.User
	if err := h.DB.First(&u, middleware.UserID(c)).Error; err != nil {
		httpx.HandleDBError(c, err)
		return
	}
	httpx.OK(c, u)
}
