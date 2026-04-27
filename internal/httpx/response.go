package httpx

import (
	"errors"
	"net/http"

	"github.com/gin-gonic/gin"
	"gorm.io/gorm"
)

type ErrorResponse struct {
	Error   string `json:"error"`
	Details any    `json:"details,omitempty"`
}

func OK(c *gin.Context, data any) {
	c.JSON(http.StatusOK, data)
}

func Created(c *gin.Context, data any) {
	c.JSON(http.StatusCreated, data)
}

func BadRequest(c *gin.Context, msg string, details any) {
	c.AbortWithStatusJSON(http.StatusBadRequest, ErrorResponse{Error: msg, Details: details})
}

func Unauthorized(c *gin.Context, msg string) {
	if msg == "" {
		msg = "unauthorized"
	}
	c.AbortWithStatusJSON(http.StatusUnauthorized, ErrorResponse{Error: msg})
}

func Forbidden(c *gin.Context, msg string) {
	if msg == "" {
		msg = "forbidden"
	}
	c.AbortWithStatusJSON(http.StatusForbidden, ErrorResponse{Error: msg})
}

func NotFound(c *gin.Context, msg string) {
	if msg == "" {
		msg = "not found"
	}
	c.AbortWithStatusJSON(http.StatusNotFound, ErrorResponse{Error: msg})
}

func Conflict(c *gin.Context, msg string) {
	c.AbortWithStatusJSON(http.StatusConflict, ErrorResponse{Error: msg})
}

func Internal(c *gin.Context, err error) {
	c.AbortWithStatusJSON(http.StatusInternalServerError, ErrorResponse{Error: "internal error", Details: err.Error()})
}

func HandleDBError(c *gin.Context, err error) bool {
	if err == nil {
		return false
	}
	if errors.Is(err, gorm.ErrRecordNotFound) {
		NotFound(c, "")
		return true
	}
	Internal(c, err)
	return true
}
