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

	_ "live-platform/docs" // Import generated docs

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/logger"
	"github.com/gofiber/fiber/v3/middleware/static"
)

// @title Live Class Streaming Platform API
// @version 1.0
// @description A complete Go Fiber v3 backend for live class streaming with PostgreSQL, Redis, MinIO, Kafka, and Nginx-RTMP
// @termsOfService http://swagger.io/terms/

// @contact.name API Support
// @contact.email support@liveplatform.com

// @license.name MIT
// @license.url https://opensource.org/licenses/MIT

// @host localhost:3000
// @BasePath /api/v1

// @securityDefinitions.apikey BearerAuth
// @in header
// @name Authorization
// @description Type "Bearer" followed by a space and JWT token.

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
	chatHandler := chat.NewHandler(chatHub, &cfg.JWT)

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

	// Swagger documentation
	app.Get("/swagger/doc.json", func(c fiber.Ctx) error {
		return c.SendFile("./docs/swagger.json")
	})
	app.Use("/web", static.New("./public"))

	// Root endpoint
	app.Get("/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "Live Platform API",
			"version": "1.0.0",
			"status":  "running",
			"docs":    "/swagger/index.html",
		})
	})

	// Health check
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})

	// API v1 routes
	api := app.Group("/api")

	// ==========================================
	// PUBLIC ROUTES (No authentication required)
	// ==========================================

	// Auth routes (public)
	authRoutes := api.Group("/auth")
	authRoutes.Post("/register/student", authHandler.RegisterStudent)
	authRoutes.Post("/register/instructor", authHandler.RegisterInstructor)
	authRoutes.Post("/login", authHandler.Login)
	authRoutes.Post("/refresh", authHandler.RefreshToken)

	// Protected auth routes
	authRoutes.Post("/logout", middleware.AuthMiddleware(&cfg.JWT), authHandler.Logout)
	authRoutes.Get("/me", middleware.AuthMiddleware(&cfg.JWT), authHandler.GetMe)
	// Admin-only: register new admin
	authRoutes.Post("/register/admin", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), authHandler.RegisterAdmin)

	// Public stream routes (anyone can view live streams)
	streams := api.Group("/streams")
	streams.Get("/live", streamHandler.ListLiveStreams)                                       // Public: view live streams
	streams.Get("/:id", middleware.OptionalAuthMiddleware(&cfg.JWT), streamHandler.GetStream) // Public: view stream details

	// ==========================================
	// PROTECTED ROUTES (Authentication required)
	// ==========================================

	// User routes (all authenticated users)
	userRoutes := api.Group("/users", middleware.AuthMiddleware(&cfg.JWT))
	userRoutes.Get("/profile", userHandler.GetProfile)                 // All authenticated users
	userRoutes.Put("/profile", userHandler.UpdateProfile)              // All authenticated users
	userRoutes.Get("/", middleware.AdminOnly(), userHandler.ListUsers) // Admin only: list all users

	// Stream management routes (instructor/admin only)
	streams.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), streamHandler.CreateStream)
	streams.Post("/:id/start", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), streamHandler.StartStream)
	streams.Post("/:id/end", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), streamHandler.EndStream)

	// Recording routes (authenticated users with role-based access)
	recordings := api.Group("/recordings")
	recordings.Get("/my", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), recordingHandler.GetMyRecordings)      // Instructor: get my recordings
	recordings.Post("/upload", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), recordingHandler.UploadRecording) // Instructor/Admin: upload recording
	recordings.Get("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.StudentOrAbove(), recordingHandler.GetRecording)           // All authenticated users
	recordings.Get("/:id/url", middleware.AuthMiddleware(&cfg.JWT), middleware.StudentOrAbove(), recordingHandler.GetRecordingURL)    // All authenticated users
	recordings.Get("/stream/:stream_id", middleware.AuthMiddleware(&cfg.JWT), middleware.StudentOrAbove(), recordingHandler.GetRecordingsByStream)

	// Chat routes (WebSocket for live chat like YouTube)
	chatRoutes := api.Group("/chat")
	chatRoutes.Get("/ws/:stream_id", chatHandler.HandleWebSocket)                                          // WebSocket auth via query param
	chatRoutes.Post("/:stream_id/send", middleware.AuthMiddleware(&cfg.JWT), chatHandler.SendMessage)      // REST fallback for sending messages
	chatRoutes.Get("/:stream_id/history", middleware.AuthMiddleware(&cfg.JWT), chatHandler.GetChatHistory) // Get chat history

	// ==========================================
	// RTMP CALLBACKS (for Nginx-RTMP server)
	// ==========================================

	// RTMP authentication handler (supports both GET and POST)
	// This is called when OBS starts streaming - auto-starts the stream
	rtmpAuthHandler := func(c fiber.Ctx) error {
		// nginx-rtmp sends stream key as 'name' parameter
		streamKey := c.Query("name")
		if streamKey == "" {
			streamKey = c.FormValue("name")
		}
		if streamKey == "" {
			log.Printf("RTMP Auth: No stream key provided")
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		// Validate and START the stream (sets status to 'live')
		stream, err := streamService.StartStreamByKey(c.Context(), streamKey)
		if err != nil {
			log.Printf("RTMP Auth: Invalid stream key: %s - %v", streamKey, err)
			return c.SendStatus(fiber.StatusUnauthorized)
		}

		log.Printf("RTMP Auth: Stream STARTED - Key: %s, Title: %s, Status: live", streamKey, stream.Title)
		return c.SendStatus(fiber.StatusOK)
	}

	// RTMP publish done handler (supports both GET and POST)
	// This is called when OBS stops streaming - auto-ends the stream
	rtmpDoneHandler := func(c fiber.Ctx) error {
		streamKey := c.Query("name")
		if streamKey == "" {
			streamKey = c.FormValue("name")
		}

		// End the stream (sets status to 'ended')
		stream, err := streamService.EndStreamByKey(c.Context(), streamKey)
		if err != nil {
			log.Printf("RTMP Done: Failed to end stream - Key: %s - %v", streamKey, err)
		} else {
			log.Printf("RTMP Done: Stream ENDED - Key: %s, Title: %s, Status: ended", streamKey, stream.Title)
		}

		return c.SendStatus(fiber.StatusOK)
	}

	// RTMP record done handler - uploads recording to MinIO
	rtmpRecordDoneHandler := func(c fiber.Ctx) error {
		streamKey := c.Query("name")
		if streamKey == "" {
			streamKey = c.FormValue("name")
		}
		recordingPath := c.Query("path")
		if recordingPath == "" {
			recordingPath = c.FormValue("path")
		}
		filename := c.Query("filename")
		if filename == "" {
			filename = c.FormValue("filename")
		}

		log.Printf("RTMP Record Done: Key=%s, Path=%s, Filename=%s", streamKey, recordingPath, filename)

		if streamKey == "" || recordingPath == "" {
			log.Printf("RTMP Record Done: Missing stream key or path")
			return c.SendStatus(fiber.StatusBadRequest)
		}

		// Upload to MinIO
		go func() {
			err := recordingService.UploadRecordingFromFile(context.Background(), streamKey, recordingPath)
			if err != nil {
				log.Printf("RTMP Record Done: Failed to upload recording - %v", err)
			} else {
				log.Printf("RTMP Record Done: Recording uploaded successfully - Key: %s", streamKey)
			}
		}()

		return c.SendStatus(fiber.StatusOK)
	}

	// Register both GET and POST for RTMP callbacks
	api.Get("/rtmp/auth", rtmpAuthHandler)
	api.Post("/rtmp/auth", rtmpAuthHandler)
	api.Get("/rtmp/done", rtmpDoneHandler)
	api.Post("/rtmp/done", rtmpDoneHandler)
	api.Get("/rtmp/record-done", rtmpRecordDoneHandler)
	api.Post("/rtmp/record-done", rtmpRecordDoneHandler)

	// Start server
	log.Printf("🚀 Server starting on port %s", cfg.Server.Port)
	log.Printf("📡 Environment: %s", cfg.Server.Env)
	log.Printf("🔗 API: http://localhost:%s", cfg.Server.Port)
	log.Printf("📚 Docs: http://localhost:%s/swagger/index.html", cfg.Server.Port)
	log.Printf("💚 Health: http://localhost:%s/health", cfg.Server.Port)

	if err := app.Listen(":" + cfg.Server.Port); err != nil {
		log.Fatal("Failed to start server:", err)
	}
}
