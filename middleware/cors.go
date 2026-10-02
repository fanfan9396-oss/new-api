package middleware

import (
	"github.com/QuantumNous/new-api/common"
	"github.com/gin-contrib/cors"
	"github.com/gin-gonic/gin"
)

// CORS is for browser-based API clients. The API uses bearer credentials rather
// than cross-site session cookies, so credentials must remain disabled while
// public API origins stay supported. Dashboard cookie requests are same-origin
// and do not depend on CORS.
func CORS() gin.HandlerFunc {
	config := cors.DefaultConfig()
	config.AllowAllOrigins = true
	config.AllowCredentials = false
	config.AllowMethods = []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"}
	config.AllowHeaders = []string{
		"Accept",
		"Authorization",
		"Cache-Control",
		"Content-Type",
		"Origin",
		"X-Auth-Session",
		"X-Requested-With",
		"X-Security-Proof",
	}
	return cors.New(config)
}

func Version() gin.HandlerFunc {
	return func(c *gin.Context) {
		c.Header("X-New-Api-Version", common.Version)
		c.Next()
	}
}
