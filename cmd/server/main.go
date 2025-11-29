package main

import (
	"context"
	"live-platform/internal/auth"
	"live-platform/internal/chat"
	"live-platform/internal/config"
	"live-platform/internal/database"
	"live-platform/internal/events"
	"live-platform/internal/middleware"
	"live-platform/internal/recording"
	"live-platform/internal/storage"
	"live-platform/internal/stream"
	"live-platform/internal/users"
	"log"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatal("Failed to load config:", err)
	}

	pgPool, err := database.NewPostgresPool(&cfg.Database)
	if err != nil {
		log.Fatal("Failed to connect to database:", err)
	}
	defer pgPool.Close()

	redisClient, err := database.NewRedisClient(&cfg.Redis)
	if err != nil {
		log.Fatal("Failed to connect to Redis:", err)
	}
	defer redisClient.Close()

	minioClient, err := storage.NewMinIOClient(&cfg.MinIO)
	if err != nil {
		log.Fatal("Failed to connect to MinIO:", err)
	}

	kafkaProducer := events.NewProducer(&cfg.Kafka)
	defer kafkaProducer.Close()

	kafkaConsumer := events.NewConsumer(&cfg.Kafka, "stream-processor")
	defer kafkaConsumer.Close()

	// Kafka event processor
	go func() {
		for {
			msg, err := kafkaConsumer.ReadMessage(context.Background())
			if err != nil {
				log.Printf("Error reading kafka message: %v", err)
				continue
			}
			log.Printf("Received event: %s", string(msg.Value))
		}
	}()

	// Initialize services
	authService := auth.NewService(pgPool, redisClient, cfg)
	authHandler := auth.NewHandler(authService)

	userService := users.NewService(pgPool)
	userHandler := users.NewHandler(userService)

	streamService := stream.NewService(pgPool, kafkaProducer)
	streamHandler := stream.NewHandler(streamService)

	recordingService := recording.NewService(pgPool, minioClient, kafkaProducer)
	recordingHandler := recording.NewHandler(recordingService)

	chatHub := chat.NewHub()
	go chatHub.Run()
	chatHandler := chat.NewHandler(chatHub)

	// Initialize Fiber app
	app := fiber.New(fiber.Config{
		AppName:      "Live Platform API",
		ServerHeader: "Live-Platform",
	})

	// Global middleware
	app.Use(logger.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "OPTIONS"},
	}))

	// Root endpoint
	app.Get("/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Live Platform API",
			"version": "1.0.0",
			"status":  "running",
		})
	})

	// Health check
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// API v1 routes
	api := app.Group("/api/v1")

	// Auth routes
	authRoutes := api.Group("/auth")
	authRoutes.Post("/register/student", authHandler.RegisterStudent)
	authRoutes.Post("/register/instructor", authHandler.RegisterInstructor)
	authRoutes.Post("/register/admin", authHandler.RegisterAdmin)
	authRoutes.Post("/login", authHandler.Login)
	authRoutes.Post("/refresh", authHandler.RefreshToken)
	authRoutes.Post("/logout", middleware.AuthMiddleware(&cfg.JWT), authHandler.Logout)

	// User routes
	userRoutes := api.Group("/users", middleware.AuthMiddleware(&cfg.JWT))
	userRoutes.Get("/profile", userHandler.GetProfile)
	userRoutes.Put("/profile", userHandler.UpdateProfile)
	userRoutes.Get("/", middleware.RoleMiddleware("admin"), userHandler.ListUsers)

	// Stream routes
	streams := api.Group("/streams")
	streams.Get("/live", streamHandler.ListLiveStreams)
	streams.Get("/:id", streamHandler.GetStream)
	streams.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.RoleMiddleware("instructor", "admin"), streamHandler.CreateStream)
	streams.Post("/:id/start", middleware.AuthMiddleware(&cfg.JWT), middleware.RoleMiddleware("instructor", "admin"), streamHandler.StartStream)
	streams.Post("/:id/end", middleware.AuthMiddleware(&cfg.JWT), middleware.RoleMiddleware("instructor", "admin"), streamHandler.EndStream)

	// Recording routes
	recordings := api.Group("/recordings", middleware.AuthMiddleware(&cfg.JWT))
	recordings.Get("/:id", recordingHandler.GetRecording)
	recordings.Get("/:id/url", recordingHandler.GetRecordingURL)
	recordings.Get("/stream/:stream_id", recordingHandler.GetRecordingsByStream)

	// Chat routes
	chatRoutes := api.Group("/chat")
	chatRoutes.Get("/ws/:stream_id", middleware.AuthMiddleware(&cfg.JWT), chatHandler.HandleWebSocket)
	chatRoutes.Post("/:stream_id", middleware.AuthMiddleware(&cfg.JWT), chatHandler.SendMessage)
	chatRoutes.Get("/:stream_id/history", middleware.AuthMiddleware(&cfg.JWT), chatHandler.GetChatHistory)

	// RTMP authentication callback (for Nginx-RTMP)
	api.Post("/rtmp/auth", func(c fiber.Ctx) error {
		streamKey := c.FormValue("name")
		if streamKey == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		// Validate stream key
		_, err := streamService.ValidateStreamKey(c.Context(), streamKey)
		if err != nil {
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		return c.SendStatus(fiber.StatusOK)
	})

	// Start server
	log.Printf("🚀 Server starting on port %s", cfg.Server.Port)
	log.Printf("📡 Environment: %s", cfg.Server.Env)
	log.Printf("🔗 API: http://localhost:%s", cfg.Server.Port)
	log.Printf("📚 Health: http://localhost:%s/health", cfg.Server.Port)
	
	if err := app.Listen(":" + cfg.Server.Port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
