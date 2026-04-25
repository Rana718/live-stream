package main

import (
	"context"
	"log/slog"
	"os"
	"os/signal"
	"syscall"
	"time"

	_ "live-platform/docs"
	"live-platform/internal/admin"
	"live-platform/internal/aiclient"
	"live-platform/internal/analytics"
	"live-platform/internal/assignments"
	"live-platform/internal/attendance"
	"live-platform/internal/audit"
	"live-platform/internal/auth"
	"live-platform/internal/banners"
	"live-platform/internal/batches"
	"live-platform/internal/bookmarks"
	"live-platform/internal/chat"
	"live-platform/internal/chapters"
	"live-platform/internal/config"
	"live-platform/internal/courseorders"
	"live-platform/internal/courses"
	"live-platform/internal/database"
	"live-platform/internal/doubts"
	"live-platform/internal/downloads"
	"live-platform/internal/enrollments"
	"live-platform/internal/events"
	"live-platform/internal/exams"
	"live-platform/internal/fees"
	"live-platform/internal/leads"
	"live-platform/internal/lectures"
	"live-platform/internal/logger"
	"live-platform/internal/materials"
	"live-platform/internal/metrics"
	"live-platform/internal/middleware"
	"live-platform/internal/notifications"
	"live-platform/internal/payments"
	"live-platform/internal/recording"
	"live-platform/internal/search"
	"live-platform/internal/storage"
	"live-platform/internal/stream"
	"live-platform/internal/sms"
	"live-platform/internal/subjects"
	"live-platform/internal/subscriptions"
	"live-platform/internal/tenants"
	"live-platform/internal/tests"
	"live-platform/internal/topics"
	"live-platform/internal/users"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/static"
	"github.com/google/uuid"
)

// @title PW-Style Live Class Streaming + Learning Platform API
// @version 2.0
// @description Full-stack edtech backend: live streaming, recorded lectures, AI doubt solving,
// @description practice tests, PYQs, analytics, subscriptions, and multi-language support.
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
		slog.Error("failed to load config", "err", err)
		os.Exit(1)
	}

	log := logger.Init(cfg.Logging.Level, cfg.Logging.Format)
	log.Info("starting server", "env", cfg.Server.Env, "port", cfg.Server.Port)

	// --- Infra connections ---
	pgPool, err := database.NewPostgresPool(&cfg.Database)
	if err != nil {
		log.Error("postgres connect failed", "err", err)
		os.Exit(1)
	}
	defer pgPool.Close()

	redisClient, err := database.NewRedisClient(&cfg.Redis)
	if err != nil {
		log.Error("redis connect failed", "err", err)
		os.Exit(1)
	}
	defer redisClient.Close()

	minioClient, err := storage.NewMinIOClient(&cfg.MinIO)
	if err != nil {
		log.Error("minio connect failed", "err", err)
		os.Exit(1)
	}

	kafkaProducer := events.NewProducer(&cfg.Kafka)
	defer kafkaProducer.Close()

	kafkaConsumer := events.NewConsumer(&cfg.Kafka, "stream-processor")
	defer kafkaConsumer.Close()

	// Background Kafka consumer
	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()
	go func() {
		for {
			select {
			case <-ctx.Done():
				return
			default:
				msg, err := kafkaConsumer.ReadMessage(ctx)
				if err != nil {
					if ctx.Err() != nil {
						return
					}
					log.Warn("kafka read error", "err", err)
					continue
				}
				log.Info("kafka event", "value", string(msg.Value))
			}
		}
	}()

	// --- Services ---
	claude := aiclient.NewClaude(cfg.Claude.APIKey, cfg.Claude.Model, cfg.Claude.MaxTokens)
	razorpay := payments.NewRazorpay(cfg.Razorpay.KeyID, cfg.Razorpay.KeySecret, cfg.Razorpay.WebhookSecret)

	// SMS provider — nil if SMS_PROVIDER is unset, in which case the OTP
	// flow runs in dev mode (logs the code, no real SMS).
	smsClient := sms.New(cfg.SMS, log)
	authService := auth.NewService(pgPool, redisClient, cfg).WithSMS(smsClient)
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

	examHandler := exams.NewHandler(exams.NewService(pgPool))
	courseHandler := courses.NewHandler(courses.NewService(pgPool))
	batchHandler := batches.NewHandler(batches.NewService(pgPool))
	enrollHandler := enrollments.NewHandler(enrollments.NewService(pgPool))
	subjectHandler := subjects.NewHandler(subjects.NewService(pgPool))
	chapterHandler := chapters.NewHandler(chapters.NewService(pgPool))
	topicHandler := topics.NewHandler(topics.NewService(pgPool))
	lectureHandler := lectures.NewHandler(lectures.NewService(pgPool))
	_ = minioClient.EnsureBucket(ctx, cfg.MinIO.MaterialsBucket)
	_ = minioClient.EnsureBucket(ctx, cfg.MinIO.DownloadsBucket)
	materialHandler := materials.NewHandler(materials.NewService(pgPool, minioClient.Raw(), cfg.MinIO.MaterialsBucket))
	testHandler := tests.NewHandler(tests.NewService(pgPool))
	doubtHandler := doubts.NewHandler(doubts.NewService(pgPool, claude))
	subsHandler := subscriptions.NewHandler(subscriptions.NewService(pgPool, razorpay), cfg.Razorpay.KeyID)
	analyticsHandler := analytics.NewHandler(analytics.NewService(pgPool))
	searchHandler := search.NewHandler(search.NewService(pgPool))
	downloadHandler := downloads.NewHandler(downloads.NewService(pgPool, minioClient.Raw(), cfg.MinIO.DownloadsBucket, cfg.App.BaseURL))

	attendanceHandler := attendance.NewHandler(attendance.NewService(pgPool))
	assignmentHandler := assignments.NewHandler(assignments.NewService(pgPool))
	notifHandler := notifications.NewHandler(notifications.NewService(pgPool))
	feesHandler := fees.NewHandler(fees.NewService(pgPool, razorpay))
	bookmarkHandler := bookmarks.NewHandler(bookmarks.NewService(pgPool))
	adminHandler := admin.NewHandler(admin.NewService(pgPool))
	auditService := audit.NewService(pgPool)
	auditHandler := audit.NewHandler(auditService)
	bannerHandler := banners.NewHandler(banners.NewService(pgPool))

	// --- Fiber app ---
	app := fiber.New(fiber.Config{
		AppName:      "PW-Style Learning Platform API",
		ServerHeader: "live-platform/2.0",
		ReadTimeout:  time.Duration(cfg.Server.ReadTimeoutSec) * time.Second,
		WriteTimeout: time.Duration(cfg.Server.WriteTimeoutSec) * time.Second,
		IdleTimeout:  time.Duration(cfg.Server.IdleTimeoutSec) * time.Second,
	})

	// Global middleware (order matters)
	app.Use(middleware.RequestID())
	app.Use(middleware.Recovery(log))
	app.Use(middleware.SecurityHeaders(cfg.TLS.Enabled))
	app.Use(metrics.Middleware())
	app.Use(middleware.RequestLogger(log))
	app.Use(middleware.LocaleMiddleware(cfg.App.DefaultLocale))
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization", "Accept-Language", "X-Request-ID"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
	}))
	if cfg.RateLimit.Enabled {
		app.Use(middleware.RateLimit(cfg.RateLimit.RequestsPerMinute, cfg.RateLimit.Burst))
	}

	// Prometheus
	app.Get("/metrics", metrics.Handler())

	// Swagger UI + JSON
	app.Get("/swagger/doc.json", func(c fiber.Ctx) error {
		return c.SendFile("./docs/swagger.json")
	})
	app.Get("/swagger", func(c fiber.Ctx) error {
		return c.Redirect().To("/swagger/index.html")
	})
	app.Get("/swagger/", func(c fiber.Ctx) error {
		return c.Redirect().To("/swagger/index.html")
	})
	app.Get("/swagger/index.html", func(c fiber.Ctx) error {
		return c.SendFile("./public/swagger-ui.html")
	})
	app.Use("/web", static.New("./public"))

	// Root + health
	app.Get("/", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"message": "PW-Style Learning Platform API",
			"version": "2.0.0",
			"docs":    "/swagger/index.html",
		})
	})
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{"status": "ok"})
	})
	app.Get("/health/deep", func(c fiber.Ctx) error {
		report := database.CollectHealth(c.Context(), pgPool, redisClient, minioClient.Raw(), cfg.Kafka.Brokers)
		status := fiber.StatusOK
		if report.Status != "ok" {
			status = fiber.StatusServiceUnavailable
		}
		return c.Status(status).JSON(report)
	})

	// --- API v1 routes ---
	api := app.Group("/api/v1")

	// --- Tenants (multi-tenant control plane) ---
	tenantSvc := tenants.NewService(pgPool)
	tenantHandler := tenants.NewHandler(tenantSvc)

	// Public Org Code lookup. No auth required — login/marketing screens
	// hit this to fetch logo + theme before issuing any JWT.
	publicGroup := api.Group("/public")
	publicGroup.Get("/tenants/by-code/:code",
		middleware.PublicLookupContext(pgPool),
		tenantHandler.LookupByOrgCode,
	)

	// Public marketing lead capture. Anyone can POST.
	leadHandler := leads.NewHandler(leads.NewService(pgPool))
	publicGroup.Post("/leads", leadHandler.Create)

	// Direct course purchase (Phase 3). Authenticated student initiates a
	// Razorpay order, then verifies the signature on success.
	courseOrderHandler := courseorders.NewHandler(
		courseorders.NewService(pgPool, razorpay),
		cfg.Razorpay.KeyID,
	)
	api.Post("/courses/:id/buy",
		middleware.AuthMiddleware(&cfg.JWT),
		middleware.TenantContext(pgPool),
		middleware.StudentOrAbove(),
		courseOrderHandler.Buy,
	)
	api.Post("/payments/verify",
		middleware.AuthMiddleware(&cfg.JWT),
		middleware.TenantContext(pgPool),
		courseOrderHandler.Verify,
	)

	// Super-admin lead triage.
	api.Get("/admin/leads",
		middleware.AuthMiddleware(&cfg.JWT),
		middleware.SuperAdminContext(pgPool),
		leadHandler.List,
	)

	// Authenticated tenant endpoints. TenantContext sets the RLS session var
	// on every request so the handler reads/writes only its tenant's rows.
	tenantsGroup := api.Group("/tenants",
		middleware.AuthMiddleware(&cfg.JWT),
		middleware.TenantContext(pgPool),
	)
	tenantsGroup.Get("/me", tenantHandler.MyTenant)
	tenantsGroup.Get("/me/features", tenantHandler.Features)
	tenantsGroup.Put("/me/branding", middleware.AdminOnly(), tenantHandler.UpdateBranding)

	// Super-admin tenant provisioning (creates new tenants).
	api.Post("/admin/tenants",
		middleware.AuthMiddleware(&cfg.JWT),
		middleware.SuperAdminContext(pgPool),
		tenantHandler.CreateTenant,
	)

	// Auth
	authRoutes := api.Group("/auth")
	authRoutes.Post("/register/student", authHandler.RegisterStudent)
	authRoutes.Post("/register/instructor", authHandler.RegisterInstructor)
	authRoutes.Post("/login", authHandler.Login)
	authRoutes.Post("/refresh", authHandler.RefreshToken)
	authRoutes.Post("/forgot-password", authHandler.ForgotPassword)
	authRoutes.Post("/reset-password", authHandler.ResetPassword)
	authRoutes.Post("/verify-email", authHandler.ConfirmEmailVerification)
	authRoutes.Post("/logout", middleware.AuthMiddleware(&cfg.JWT), authHandler.Logout)
	authRoutes.Get("/me", middleware.AuthMiddleware(&cfg.JWT), authHandler.GetMe)
	authRoutes.Post("/verify-email/start", middleware.AuthMiddleware(&cfg.JWT), authHandler.SendEmailVerification)
	authRoutes.Post("/register/admin", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), authHandler.RegisterAdmin)

	// Mobile OTP + Google sign-in. `otp/send` and `otp/verify` are public so
	// brand-new users can establish accounts; `link/*` require a bearer token
	// because they mutate an existing identity.
	authRoutes.Post("/otp/send", authHandler.SendOtp)
	authRoutes.Post("/otp/verify", authHandler.VerifyOtp)
	authRoutes.Post("/google", authHandler.GoogleSignIn)
	authRoutes.Post("/link/phone", middleware.AuthMiddleware(&cfg.JWT), authHandler.LinkPhone)
	authRoutes.Post("/link/google", middleware.AuthMiddleware(&cfg.JWT), authHandler.LinkGoogle)

	// Users
	userRoutes := api.Group("/users", middleware.AuthMiddleware(&cfg.JWT))
	userRoutes.Get("/profile", userHandler.GetProfile)
	userRoutes.Put("/profile", userHandler.UpdateProfile)
	userRoutes.Post("/me/onboarding", userHandler.CompleteOnboarding)
	userRoutes.Get("/", middleware.AdminOnly(), userHandler.ListUsers)

	// Streaming (existing)
	streams := api.Group("/streams")
	streams.Get("/live", streamHandler.ListLiveStreams)
	streams.Get("/:id", middleware.OptionalAuthMiddleware(&cfg.JWT), streamHandler.GetStream)
	streams.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), streamHandler.CreateStream)
	streams.Post("/:id/start", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), streamHandler.StartStream)
	streams.Post("/:id/end", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), streamHandler.EndStream)

	// Recordings (existing)
	recs := api.Group("/recordings")
	recs.Get("/my", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), recordingHandler.GetMyRecordings)
	recs.Post("/upload", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), recordingHandler.UploadRecording)
	recs.Get("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.StudentOrAbove(), recordingHandler.GetRecording)
	recs.Get("/:id/url", middleware.AuthMiddleware(&cfg.JWT), middleware.StudentOrAbove(), recordingHandler.GetRecordingURL)
	recs.Get("/stream/:stream_id", middleware.AuthMiddleware(&cfg.JWT), middleware.StudentOrAbove(), recordingHandler.GetRecordingsByStream)

	// Chat (existing)
	chatRoutes := api.Group("/chat")
	chatRoutes.Get("/ws/:stream_id", chatHandler.HandleWebSocket)
	chatRoutes.Post("/:stream_id/send", middleware.AuthMiddleware(&cfg.JWT), chatHandler.SendMessage)
	chatRoutes.Get("/:stream_id/history", middleware.AuthMiddleware(&cfg.JWT), chatHandler.GetChatHistory)

	// Exam categories
	ec := api.Group("/exam-categories")
	ec.Get("/", examHandler.List)
	ec.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), examHandler.Create)
	ec.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), examHandler.Update)
	ec.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), examHandler.Delete)

	// Courses
	cg := api.Group("/courses")
	cg.Get("/", courseHandler.List)
	cg.Get("/:id", courseHandler.Get)
	cg.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), courseHandler.Create)
	cg.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), courseHandler.Update)
	cg.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), courseHandler.Delete)

	// Batches
	bg := api.Group("/batches")
	bg.Get("/course/:course_id", batchHandler.ListByCourse)
	bg.Get("/my", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), batchHandler.ListMine)
	bg.Get("/:id", batchHandler.Get)
	bg.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), batchHandler.Create)
	bg.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), batchHandler.Update)
	bg.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), batchHandler.Delete)

	// Enrollments
	eg := api.Group("/enrollments", middleware.AuthMiddleware(&cfg.JWT))
	eg.Post("/", enrollHandler.Enroll)
	eg.Get("/my", enrollHandler.ListMine)
	eg.Delete("/:course_id", enrollHandler.Cancel)

	// Subjects / Chapters / Topics
	subj := api.Group("/subjects")
	subj.Get("/course/:course_id", subjectHandler.ListByCourse)
	subj.Get("/:id", subjectHandler.Get)
	subj.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), subjectHandler.Create)
	subj.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), subjectHandler.Update)
	subj.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), subjectHandler.Delete)

	ch := api.Group("/chapters")
	ch.Get("/subject/:subject_id", chapterHandler.ListBySubject)
	ch.Get("/:id", chapterHandler.Get)
	ch.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), chapterHandler.Create)
	ch.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), chapterHandler.Update)
	ch.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), chapterHandler.Delete)

	tp := api.Group("/topics")
	tp.Get("/chapter/:chapter_id", topicHandler.ListByChapter)
	tp.Get("/:id", topicHandler.Get)
	tp.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), topicHandler.Create)
	tp.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), topicHandler.Update)
	tp.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), topicHandler.Delete)

	// Lectures
	lg := api.Group("/lectures")
	lg.Get("/", lectureHandler.List)
	lg.Get("/:id", lectureHandler.Get)
	lg.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), lectureHandler.Create)
	lg.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), lectureHandler.Update)
	lg.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), lectureHandler.Delete)
	lg.Post("/watch", middleware.AuthMiddleware(&cfg.JWT), lectureHandler.RecordWatch)
	lg.Get("/history/my", middleware.AuthMiddleware(&cfg.JWT), lectureHandler.History)

	// Study materials
	mg := api.Group("/materials")
	mg.Post("/upload", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), materialHandler.Upload)
	mg.Get("/chapter/:chapter_id", materialHandler.ListByChapter)
	mg.Get("/topic/:topic_id", materialHandler.ListByTopic)
	mg.Get("/:id", materialHandler.Get)
	mg.Get("/:id/download", middleware.AuthMiddleware(&cfg.JWT), materialHandler.GetDownloadURL)
	mg.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), materialHandler.Delete)

	// Tests / Questions / Attempts (DPPs + PYQs share this API via test_type)
	tg := api.Group("/tests")
	tg.Get("/", testHandler.ListTests)
	tg.Get("/:id", middleware.AuthMiddleware(&cfg.JWT), testHandler.GetTest)
	tg.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), testHandler.CreateTest)
	tg.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), testHandler.UpdateTest)
	tg.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), testHandler.DeleteTest)
	tg.Post("/questions", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), testHandler.CreateQuestion)
	tg.Delete("/questions/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), testHandler.DeleteQuestion)
	tg.Post("/:id/attempts", middleware.AuthMiddleware(&cfg.JWT), testHandler.StartAttempt)
	tg.Post("/attempts/answer", middleware.AuthMiddleware(&cfg.JWT), testHandler.SubmitAnswer)
	tg.Post("/attempts/:id/submit", middleware.AuthMiddleware(&cfg.JWT), testHandler.SubmitAttempt)
	tg.Get("/attempts/my", middleware.AuthMiddleware(&cfg.JWT), testHandler.ListMyAttempts)
	tg.Get("/attempts/:id", middleware.AuthMiddleware(&cfg.JWT), testHandler.GetAttempt)

	// Doubts
	dg := api.Group("/doubts", middleware.AuthMiddleware(&cfg.JWT))
	dg.Post("/", doubtHandler.Ask)
	dg.Get("/my", doubtHandler.ListMine)
	dg.Get("/pending", middleware.InstructorOrAdmin(), doubtHandler.ListPending)
	dg.Get("/lecture/:lecture_id", doubtHandler.ListByLecture)
	dg.Get("/:id", doubtHandler.Get)
	dg.Post("/answer", middleware.InstructorOrAdmin(), doubtHandler.InstructorAnswer)
	dg.Post("/answers/:id/accept", doubtHandler.AcceptAnswer)
	dg.Delete("/:id", doubtHandler.Delete)

	// Subscriptions + payments
	sg := api.Group("/subscriptions")
	sg.Get("/plans", subsHandler.ListPlans)
	sg.Post("/plans", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), subsHandler.CreatePlan)
	sg.Post("/checkout", middleware.AuthMiddleware(&cfg.JWT), subsHandler.Checkout)
	sg.Post("/verify", middleware.AuthMiddleware(&cfg.JWT), subsHandler.Verify)
	sg.Get("/me", middleware.AuthMiddleware(&cfg.JWT), subsHandler.GetMine)
	sg.Get("/history", middleware.AuthMiddleware(&cfg.JWT), subsHandler.ListMyHistory)
	sg.Post("/:id/cancel", middleware.AuthMiddleware(&cfg.JWT), subsHandler.Cancel)
	sg.Post("/webhook", subsHandler.Webhook)

	// Analytics
	ag := api.Group("/analytics", middleware.AuthMiddleware(&cfg.JWT))
	ag.Get("/me", analyticsHandler.GetMyStats)
	ag.Get("/weak-topics", analyticsHandler.GetWeakTopics)
	ag.Get("/difficulty", analyticsHandler.GetDifficultyBreakdown)
	ag.Get("/recent-attempts", analyticsHandler.GetRecentAttempts)

	// Search
	api.Get("/search", searchHandler.Search)

	// Home-page banners (public read; admin CRUD lives under /admin/banners).
	api.Get("/banners", bannerHandler.ListActive)

	// Downloads / video variants / offline tokens
	dl := api.Group("/downloads")
	dl.Post("/variants", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), downloadHandler.CreateVariant)
	dl.Get("/lectures/:lecture_id/variants", downloadHandler.ListVariantsForLecture)
	dl.Post("/token", middleware.AuthMiddleware(&cfg.JWT), downloadHandler.IssueToken)
	dl.Get("/fetch", downloadHandler.Fetch)

	// Attendance
	att := api.Group("/attendance", middleware.AuthMiddleware(&cfg.JWT))
	att.Post("/auto", attendanceHandler.AutoMark)
	att.Post("/manual", middleware.InstructorOrAdmin(), attendanceHandler.ManualMark)
	att.Post("/lecture/:id/bulk", middleware.InstructorOrAdmin(), attendanceHandler.BulkMark)
	att.Get("/lecture/:id", middleware.InstructorOrAdmin(), attendanceHandler.ListByLecture)
	att.Get("/my", attendanceHandler.ListMine)
	att.Get("/my/stats", attendanceHandler.GetMyStats)
	att.Get("/my/subjects", attendanceHandler.GetMySubjectBreakdown)
	att.Get("/my/monthly", attendanceHandler.MonthlyReport)
	att.Get("/low", middleware.InstructorOrAdmin(), attendanceHandler.LowAttendance)
	att.Get("/batch/:id/export", middleware.InstructorOrAdmin(), attendanceHandler.ExportCSV)
	att.Post("/qr/:lecture_id", middleware.InstructorOrAdmin(), attendanceHandler.CreateQRCode)
	att.Post("/qr/check-in", attendanceHandler.QRCheckIn)

	// Assignments
	asg := api.Group("/assignments")
	asg.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), assignmentHandler.Create)
	asg.Get("/mine", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), assignmentHandler.ListMine)
	asg.Get("/my-submissions", middleware.AuthMiddleware(&cfg.JWT), assignmentHandler.ListMySubmissions)
	asg.Post("/submit", middleware.AuthMiddleware(&cfg.JWT), assignmentHandler.Submit)
	asg.Post("/submissions/:id/grade", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), assignmentHandler.Grade)
	asg.Get("/batch/:batch_id", middleware.AuthMiddleware(&cfg.JWT), assignmentHandler.ListByBatch)
	asg.Get("/course/:course_id", middleware.AuthMiddleware(&cfg.JWT), assignmentHandler.ListByCourse)
	asg.Get("/:id", middleware.AuthMiddleware(&cfg.JWT), assignmentHandler.Get)
	asg.Put("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), assignmentHandler.Update)
	asg.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), assignmentHandler.Delete)
	asg.Get("/:id/submissions", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), assignmentHandler.ListSubmissions)
	asg.Get("/:id/my-submission", middleware.AuthMiddleware(&cfg.JWT), assignmentHandler.GetMySubmission)

	// Notifications
	ng := api.Group("/notifications", middleware.AuthMiddleware(&cfg.JWT))
	ng.Get("/", notifHandler.ListMine)
	ng.Get("/unread-count", notifHandler.UnreadCount)
	ng.Post("/read-all", notifHandler.MarkAllRead)
	ng.Post("/:id/read", notifHandler.MarkRead)
	ng.Delete("/:id", notifHandler.Delete)

	// Announcements
	anc := api.Group("/announcements")
	anc.Get("/", notifHandler.ListGlobal)
	anc.Get("/batch/:batch_id", notifHandler.ListBatch)
	anc.Get("/course/:course_id", notifHandler.ListCourse)
	anc.Post("/", middleware.AuthMiddleware(&cfg.JWT), middleware.InstructorOrAdmin(), notifHandler.CreateAnnouncement)
	anc.Delete("/:id", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly(), notifHandler.DeleteAnnouncement)

	// Fees
	fg := api.Group("/fees", middleware.AuthMiddleware(&cfg.JWT))
	fg.Post("/structures", middleware.AdminOnly(), feesHandler.CreateStructure)
	fg.Get("/structures/course/:course_id", feesHandler.ListStructuresByCourse)
	fg.Post("/assign", middleware.AdminOnly(), feesHandler.Assign)
	fg.Get("/my", feesHandler.ListMine)
	fg.Get("/pending", middleware.AdminOnly(), feesHandler.ListPending)
	fg.Get("/installments/overdue", middleware.AdminOnly(), feesHandler.ListOverdueInstallments)
	fg.Post("/installments/pay", feesHandler.PayInstallment)
	fg.Post("/installments/verify", feesHandler.VerifyInstallment)
	fg.Get("/revenue", middleware.AdminOnly(), feesHandler.Revenue)
	fg.Get("/:id/installments", feesHandler.GetInstallments)

	// Bookmarks
	bm := api.Group("/bookmarks", middleware.AuthMiddleware(&cfg.JWT))
	bm.Post("/", bookmarkHandler.Create)
	bm.Get("/", bookmarkHandler.ListMine)
	bm.Get("/lecture/:lecture_id", bookmarkHandler.ListForLecture)
	bm.Delete("/:id", bookmarkHandler.Delete)

	// Admin
	adm := api.Group("/admin", middleware.AuthMiddleware(&cfg.JWT), middleware.AdminOnly())
	adm.Get("/dashboard", adminHandler.Dashboard)
	adm.Get("/users", adminHandler.ListUsers)
	adm.Get("/users/export", adminHandler.ExportUsersCSV)
	adm.Put("/users/:id", adminHandler.UpdateUser)
	adm.Post("/users/:id/role", adminHandler.SetUserRole)
	adm.Post("/users/:id/active", adminHandler.SetUserActive)
	adm.Post("/users/:id/password", adminHandler.ResetUserPassword)
	adm.Delete("/users/:id", adminHandler.DeleteUser)
	adm.Get("/attendance/batches", adminHandler.BatchAttendance)
	adm.Get("/courses/pending", adminHandler.ListPendingApproval)
	adm.Post("/courses/:id/approve", adminHandler.ApproveCourse)
	adm.Post("/courses/:id/reject", adminHandler.RejectCourse)
	adm.Post("/notifications/send", notifHandler.AdminSend)
	adm.Get("/audit", auditHandler.List)
	adm.Get("/banners", bannerHandler.ListAll)
	adm.Post("/banners", bannerHandler.Create)
	adm.Put("/banners/:id", bannerHandler.Update)
	adm.Post("/banners/:id/active", bannerHandler.SetActive)
	adm.Delete("/banners/:id", bannerHandler.Delete)
	// Attach audit middleware last so it only records admin-scope mutations.
	adm.Use(middleware.Audit(auditService))

	// RTMP callbacks
	rtmpAuthHandler := func(c fiber.Ctx) error {
		key := c.Query("name")
		if key == "" {
			key = c.FormValue("name")
		}
		if key == "" {
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		if _, err := streamService.StartStreamByKey(c.Context(), key); err != nil {
			log.Warn("rtmp invalid key", "key", key, "err", err)
			return c.SendStatus(fiber.StatusUnauthorized)
		}
		log.Info("rtmp stream started", "key", key)
		return c.SendStatus(fiber.StatusOK)
	}
	rtmpDoneHandler := func(c fiber.Ctx) error {
		key := c.Query("name")
		if key == "" {
			key = c.FormValue("name")
		}
		if _, err := streamService.EndStreamByKey(c.Context(), key); err != nil {
			log.Warn("rtmp end failed", "key", key, "err", err)
		}
		return c.SendStatus(fiber.StatusOK)
	}
	rtmpRecordDone := func(c fiber.Ctx) error {
		key := c.Query("name")
		if key == "" {
			key = c.FormValue("name")
		}
		path := c.Query("path")
		if path == "" {
			path = c.FormValue("path")
		}
		if key == "" || path == "" {
			return c.SendStatus(fiber.StatusBadRequest)
		}
		go func() {
			if err := recordingService.UploadRecordingFromFile(context.Background(), key, path); err != nil {
				log.Error("recording upload failed", "err", err)
			}
		}()
		return c.SendStatus(fiber.StatusOK)
	}
	api.Get("/rtmp/auth", rtmpAuthHandler)
	api.Post("/rtmp/auth", rtmpAuthHandler)
	api.Get("/rtmp/done", rtmpDoneHandler)
	api.Post("/rtmp/done", rtmpDoneHandler)
	api.Get("/rtmp/record-done", rtmpRecordDone)
	api.Post("/rtmp/record-done", rtmpRecordDone)

	// --- Graceful shutdown ---
	go func() {
		addr := ":" + cfg.Server.Port
		listenCfg := fiber.ListenConfig{DisableStartupMessage: false}
		if cfg.TLS.Enabled {
			listenCfg.CertFile = cfg.TLS.CertFile
			listenCfg.CertKeyFile = cfg.TLS.KeyFile
			log.Info("listening with TLS", "addr", addr)
		} else {
			log.Info("listening", "addr", addr)
		}
		if err := app.Listen(addr, listenCfg); err != nil {
			log.Error("server exited", "err", err)
			cancel()
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	sig := <-quit
	log.Info("shutdown signal received", "signal", sig.String())

	shutdownCtx, shutdownCancel := context.WithTimeout(context.Background(), time.Duration(cfg.Server.ShutdownTimeout)*time.Second)
	defer shutdownCancel()
	if err := app.ShutdownWithContext(shutdownCtx); err != nil {
		log.Error("shutdown error", "err", err)
	}
	cancel()
	log.Info("server stopped cleanly")

	// Prevent unused imports if we strip handlers in the future.
	_ = uuid.Nil
}
