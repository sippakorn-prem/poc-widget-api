package main

import (
	"net/http"
	"os"
	"strings"

	"github.com/labstack/echo/v4"
	"github.com/labstack/echo/v4/middleware"
)

func main() {
	widgetOrigin := env("WIDGET_ORIGIN", "http://localhost:5173")
	allowedHosts := splitEnv("ALLOWED_HOSTS", "http://localhost:5174")

	e := echo.New()
	e.HideBanner = true

	// The iframe document calls the API from the widget origin; the customer
	// origin is checked separately via the bootstrap host parameter.
	e.Use(middleware.CORSWithConfig(middleware.CORSConfig{
		AllowOriginFunc: func(origin string) (bool, error) {
			return origin == widgetOrigin || strings.HasPrefix(origin, "http://localhost"), nil
		},
		AllowMethods: []string{http.MethodGet},
	}))

	e.GET("/", func(c echo.Context) error {
		return c.String(http.StatusOK, "ok")
	})

	e.GET("/widget/v1/bootstrap", func(c echo.Context) error {
		if c.QueryParam("key") != "wk_demo" {
			return c.JSON(http.StatusNotFound, map[string]string{"error": "unknown key"})
		}

		host := c.QueryParam("host")
		if !contains(allowedHosts, host) {
			return c.JSON(http.StatusForbidden, map[string]string{"error": "origin not allowed"})
		}

		return c.JSON(http.StatusOK, map[string]any{
			"displayName": "ZimpleOmni",
			"color":       "#4f46e5",
			"welcome":     "Served cross-origin and gated by the POC API.",
		})
	})

	e.Logger.Fatal(e.Start(":" + env("PORT", "8080")))
}

func env(key, fallback string) string {
	if value := strings.TrimSpace(os.Getenv(key)); value != "" {
		return value
	}
	return fallback
}

func splitEnv(key, fallback string) []string {
	raw := env(key, fallback)
	parts := strings.Split(raw, ",")
	out := make([]string, 0, len(parts))
	for _, part := range parts {
		if trimmed := strings.TrimSpace(part); trimmed != "" {
			out = append(out, trimmed)
		}
	}
	return out
}

func contains(values []string, target string) bool {
	for _, value := range values {
		if value == target {
			return true
		}
	}
	return false
}
