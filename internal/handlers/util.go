package handlers

import (
	"strconv"

	"github.com/ahmad/smart-inventory/internal/httpx"
	"github.com/gin-gonic/gin"
)

// parseUintParam grabs a uint from the URL or writes a 400.
func parseUintParam(c *gin.Context, name string) (uint, bool) {
	raw := c.Param(name)
	v, err := strconv.ParseUint(raw, 10, 64)
	if err != nil {
		httpx.BadRequest(c, "invalid "+name, raw)
		return 0, false
	}
	return uint(v), true
}
