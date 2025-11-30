package middleware

import (
	"live-platform/internal/config"
	"live-platform/internal/utils"
	"log"
	"strings"

	"github.com/gofiber/fiber/v3"
)

// AuthMiddleware checks for JWT token in Authorization header
func AuthMiddleware(cfg *config.JWTConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		var token string

		// Check Authorization header
		authHeader := c.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		if token == "" {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "missing authentication token",
			})
		}

		claims, err := utils.ValidateToken(token, cfg.AccessSecret)
		if err != nil {
			return c.Status(fiber.StatusUnauthorized).JSON(fiber.Map{
				"error": "invalid or expired token",
			})
		}

		c.Locals("userID", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		c.Locals("username", claims.Email)

		return c.Next()
	}
}

// OptionalAuthMiddleware checks for JWT but allows unauthenticated requests
func OptionalAuthMiddleware(cfg *config.JWTConfig) fiber.Handler {
	return func(c fiber.Ctx) error {
		var token string

		// Check Authorization header
		authHeader := c.Get("Authorization")
		if authHeader != "" {
			parts := strings.Split(authHeader, " ")
			if len(parts) == 2 && parts[0] == "Bearer" {
				token = parts[1]
			}
		}

		// If no token, continue without authentication
		if token == "" {
			return c.Next()
		}

		// If token exists, validate it
		claims, err := utils.ValidateToken(token, cfg.AccessSecret)
		if err != nil {
			// Invalid token, continue without authentication
			return c.Next()
		}

		c.Locals("userID", claims.UserID)
		c.Locals("email", claims.Email)
		c.Locals("role", claims.Role)
		c.Locals("username", claims.Email)

		return c.Next()
	}
}

// RoleMiddleware checks if the user has one of the allowed roles
func RoleMiddleware(allowedRoles ...string) fiber.Handler {
	return func(c fiber.Ctx) error {
		role, ok := c.Locals("role").(string)
		if !ok {
			log.Printf("[RoleMiddleware] Role not found in context")
			return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
				"error": "role not found in context",
			})
		}

		// Normalize role to lowercase for comparison
		normalizedRole := strings.ToLower(strings.TrimSpace(role))
		log.Printf("[RoleMiddleware] User role: '%s' (normalized: '%s'), Allowed roles: %v", role, normalizedRole, allowedRoles)

		for _, allowedRole := range allowedRoles {
			if normalizedRole == strings.ToLower(strings.TrimSpace(allowedRole)) {
				return c.Next()
			}
		}

		log.Printf("[RoleMiddleware] Access denied for role '%s'", role)
		return c.Status(fiber.StatusForbidden).JSON(fiber.Map{
			"error": "insufficient permissions",
		})
	}
}

// AdminOnly middleware - only admins can access
func AdminOnly() fiber.Handler {
	return RoleMiddleware("admin")
}

// InstructorOrAdmin middleware - instructors and admins can access
func InstructorOrAdmin() fiber.Handler {
	return RoleMiddleware("instructor", "admin")
}

// StudentOrAbove middleware - all authenticated users can access
func StudentOrAbove() fiber.Handler {
	return RoleMiddleware("student", "instructor", "admin")
}
