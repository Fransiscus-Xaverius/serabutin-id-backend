package main

import (
	"context"
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"
	"time"

	"github.com/gofiber/fiber/v3"
	"github.com/gofiber/fiber/v3/middleware/compress"
	"github.com/gofiber/fiber/v3/middleware/cors"
	"github.com/gofiber/fiber/v3/middleware/helmet"
	"github.com/gofiber/fiber/v3/middleware/recover"
	"github.com/gofiber/fiber/v3/middleware/requestid"
	staticmiddleware "github.com/gofiber/fiber/v3/middleware/static"

	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
	"github.com/serabutin/serabutin-id-backend/internal/handler"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/chatclient"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/config"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/database"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/imaging"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/logger"
	infraredis "github.com/serabutin/serabutin-id-backend/internal/infrastructure/redis"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/storage"
	"github.com/serabutin/serabutin-id-backend/internal/middleware"
	gormrepo "github.com/serabutin/serabutin-id-backend/internal/repository/gorm"
	localrepo "github.com/serabutin/serabutin-id-backend/internal/repository/local"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	redisrepo "github.com/serabutin/serabutin-id-backend/internal/repository/redis"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
)

func main() {
	cfg, err := config.Load()
	if err != nil {
		log.Fatalf("failed to load config: %v", err)
	}

	logger.Init(cfg.Environment)

	logger.Log.Info().
		Str("app", cfg.AppName).
		Str("environment", cfg.Environment).
		Msg("starting application")

	db, err := database.NewPostgresDB(cfg.DatabaseURL)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to database")
	}

	redisClient, err := infraredis.NewRedisClient(cfg.RedisAddr, cfg.RedisPassword, cfg.RedisDB)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to connect to redis")
	}

	// Reviews v2 prep: add the new columns as nullable-with-default and backfill
	// legacy rows (author = customer) BEFORE AutoMigrate enforces NOT NULL,
	// otherwise the migration fails on existing data.
	db.Exec(`ALTER TABLE reviews ADD COLUMN IF NOT EXISTS author_id bigint DEFAULT 0`)
	db.Exec(`ALTER TABLE reviews ADD COLUMN IF NOT EXISTS author_role varchar(20) DEFAULT 'customer'`)
	db.Exec(`UPDATE reviews SET author_id = customer_id WHERE author_id = 0 OR author_id IS NULL`)
	db.Exec(`UPDATE reviews SET author_role = 'customer' WHERE author_role = '' OR author_role IS NULL`)
	// Bid-derived bookings have no schedule until the provider accepts them;
	// relax the legacy NOT NULL constraints before AutoMigrate runs.
	db.Exec(`ALTER TABLE bookings ALTER COLUMN schedule_date DROP NOT NULL`)
	db.Exec(`ALTER TABLE bookings ALTER COLUMN schedule_time DROP NOT NULL`)
	db.Exec(`ALTER TABLE bookings ALTER COLUMN address DROP NOT NULL`)
	// Ticket 3: suspend flag for existing deployments created before the
	// status column (AutoMigrate below only adds it on fresh tables).
	db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS status varchar(20) NOT NULL DEFAULT 'active'`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_status ON users (status)`)
	// Force-cancel writes PolicyVersion "admin-override" (14 chars) into
	// cancellations.policy_version varchar(10) and "admin_override:"+reason
	// into cancellations.reason varchar(30) — every force-cancel 500s with
	// "value too long". AutoMigrate does NOT resize existing Postgres
	// columns, so widen them here (plain ALTER is idempotent when the type
	// already matches; safe no-op on fresh tables created wide by migrate).
	db.Exec(`ALTER TABLE cancellations ALTER COLUMN policy_version TYPE varchar(32)`)
	db.Exec(`ALTER TABLE cancellations ALTER COLUMN reason TYPE varchar(500)`)
	// Ticket 6: moderation flag for existing deployments created before the
	// hidden column (AutoMigrate below only adds it on fresh tables).
	db.Exec(`ALTER TABLE reviews ADD COLUMN IF NOT EXISTS hidden boolean NOT NULL DEFAULT false`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_reviews_hidden ON reviews (hidden)`)
	// Ticket 8: chat-only block flag for existing deployments created
	// before the column (AutoMigrate below only adds it on fresh tables).
	db.Exec(`ALTER TABLE users ADD COLUMN IF NOT EXISTS chat_blocked boolean NOT NULL DEFAULT false`)
	db.Exec(`CREATE INDEX IF NOT EXISTS idx_users_chat_blocked ON users (chat_blocked)`)
	if err := db.AutoMigrate(&model.UserModel{}, &model.ServiceModel{}, &model.ServiceImageModel{}, &model.ServiceAddOnModel{}, &model.BookingModel{}, &model.BookingAddOnModel{}, &model.ReviewModel{}, &model.BidModel{}, &model.NotificationModel{}, &model.CancellationModel{}, &model.AuditLogModel{}, &model.ReportModel{}, &model.BroadcastModel{}, &model.BannerModel{}, &model.CategoryModel{}, &model.ReimbursementModel{}, &model.PayoutMarkModel{}, &model.KillSwitchModel{}); err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to run auto migration")
	}
	// Drop the legacy one-review-per-booking unique index (replaced by the
	// composite booking+author index).
	if db.Migrator().HasIndex(&model.ReviewModel{}, "idx_reviews_booking_id") {
		db.Migrator().DropIndex(&model.ReviewModel{}, "idx_reviews_booking_id")
	}
	logger.Log.Info().Msg("database migrated successfully")

	storageRepo, err := storage.NewStorageRepository(cfg)
	if err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to create storage repository")
	}

	userRepo := gormrepo.NewUserRepository(db)
	serviceRepo := gormrepo.NewServiceRepository(db)
	bookingRepo := gormrepo.NewBookingRepository(db)
	reviewRepo := gormrepo.NewReviewRepository(db)
	tokenRepo := redisrepo.NewTokenRepository(redisClient)
	tokenService := service.NewTokenService(cfg.JWTSecret, cfg.JWTRefreshSecret, cfg.AccessTokenDuration, cfg.RefreshTokenDuration)
	loginLimiter := service.NewLoginLimiter(redisClient, cfg.LoginMaxAttempts, time.Duration(cfg.LoginLockoutMinutes)*time.Minute)

	authUsecase := usecase.NewAuthUsecase(userRepo, tokenRepo, tokenService, loginLimiter, cfg.BcryptCost)
	authHandler := handler.NewAuthHandler(authUsecase)
	profileUsecase := usecase.NewProfileUsecase(userRepo)
	profileHandler := handler.NewProfileHandler(profileUsecase)
	serviceUsecase := usecase.NewServiceUsecase(serviceRepo, userRepo, reviewRepo)
	serviceHandler := handler.NewServiceHandler(serviceUsecase, profileUsecase, storageRepo)
	bookingUsecase := usecase.NewBookingUsecase(bookingRepo, serviceRepo, userRepo, cfg.PlatformFee, cfg.CancellationFeeCustomer, cfg.CancellationFeeProvider, gormrepo.NewCancellationRepository(db))
	bookingHandler := handler.NewBookingHandler(bookingUsecase, profileUsecase, serviceUsecase)
	bidRepo := gormrepo.NewBidRepository(db)
	bidUsecase := usecase.NewBidUsecase(bidRepo, serviceRepo, bookingRepo, userRepo, cfg.PlatformFee)
	bidHandler := handler.NewBidHandler(bidUsecase, profileUsecase, serviceUsecase)
	reviewUsecase := usecase.NewReviewUsecase(reviewRepo, bookingRepo, userRepo)
	reviewHandler := handler.NewReviewHandler(reviewUsecase, profileUsecase)
	introspectUsecase := usecase.NewIntrospectUsecase(tokenService, tokenRepo, userRepo)
	introspectHandler := handler.NewIntrospectHandler(introspectUsecase)
	userUsecase := usecase.NewUserUsecase(userRepo)
	statsUsecase := usecase.NewStatsUsecase(userRepo, serviceRepo, bookingRepo)
	userHandler := handler.NewUserHandler(userUsecase, statsUsecase, reviewUsecase)
	notificationRepo := gormrepo.NewNotificationRepository(db)
	notificationUsecase := usecase.NewNotificationUsecase(notificationRepo)
	notificationHandler := handler.NewNotificationHandler(notificationUsecase)
	bookingUsecase.SetNotifier(notificationUsecase)
	bidUsecase.SetNotifier(notificationUsecase)
	cancelRepo := gormrepo.NewCancellationRepository(db)
	auditRepo := gormrepo.NewAuditLogRepository(db)
	adminUserUsecase := usecase.NewAdminUserUsecase(userRepo)
	// F1: Suspend revokes every live token via the Redis token store.
	adminUserUsecase.SetTokenRepository(tokenRepo)
	adminServiceUsecase := usecase.NewAdminServiceUsecase(serviceRepo)
	adminBookingUsecase := usecase.NewAdminBookingUsecase(bookingRepo, cancelRepo)
	adminBookingUsecase.SetNotifier(notificationUsecase)
	adminReviewUsecase := usecase.NewAdminReviewUsecase(reviewRepo, gormrepo.NewReportRepository(db))
	adminStatsUsecase := usecase.NewAdminStatsUsecase(gormrepo.NewAdminStatsRepository(db))
	adminChatUsecase := usecase.NewAdminChatUsecase(userRepo, chatclient.NewClient(cfg.ChatBackendBaseURL, cfg.ChatServiceToken, 5*time.Second))
	adminHandler := handler.NewAdminHandler(cancelRepo, notificationUsecase, bookingRepo, profileUsecase, serviceUsecase, auditRepo, adminUserUsecase, adminServiceUsecase, storageRepo, adminBookingUsecase)
	adminHandler.SetAdminReviewUC(adminReviewUsecase)
	adminHandler.SetAdminStatsUC(adminStatsUsecase)
	adminHandler.SetAdminChatUC(adminChatUsecase)
	broadcastRepo := gormrepo.NewBroadcastRepository(db)
	adminBroadcastUC := usecase.NewAdminBroadcastUsecase(broadcastRepo, notificationRepo)
	adminHandler.SetAdminBroadcastUC(adminBroadcastUC)
	// Ticket 12: kill-switch hybrid store (DB durable intent + Redis
	// runtime). Admin writes land in DB first, then publish to Redis; boot
	// seeds all-enabled rows and reloads Redis from DB. Failures here are
	// logged, never fatal — hot paths fail open without runtime state.
	killSwitchUC := usecase.NewKillSwitchUsecase(
		gormrepo.NewKillSwitchRepository(db),
		usecase.NewRedisKillSwitchCache(redisClient),
	)
	if err := killSwitchUC.EnsureSeeded(context.Background()); err != nil {
		logger.Log.Error().Err(err).Msg("failed to seed kill switches")
	}
	if err := killSwitchUC.SyncToRedis(context.Background()); err != nil {
		logger.Log.Error().Err(err).Msg("failed to sync kill switches to redis")
	}
	adminHandler.SetKillSwitchUC(killSwitchUC)
	adminBroadcastUC.SetKillSwitchGate(killSwitchUC)
	introspectUsecase.SetKillSwitchGate(killSwitchUC)
	// Ticket 10: CMS taxonomy + banners. The taxonomy check is attached to
	// the provider service flow so create/update validates against active
	// category rows; the startup seed below backfills live values first so
	// enforcement never breaks existing listings.
	bannerRepo := gormrepo.NewBannerRepository(db)
	categoryRepo := gormrepo.NewCategoryRepository(db)
	adminCMSUC := usecase.NewAdminCMSUsecase(bannerRepo, categoryRepo)
	adminHandler.SetAdminCMSUC(adminCMSUC)
	cmsHandler := handler.NewCmsHandler(adminCMSUC)
	// Ticket 11: finance overview (derived from bookings) + provider-filed
	// reimbursements with admin approve/reject as decision-record only.
	financeRepo := gormrepo.NewFinanceRepository(db)
	reimbRepo := gormrepo.NewReimbursementRepository(db)
	adminFinanceUC := usecase.NewAdminFinanceUsecase(financeRepo, reimbRepo, bookingRepo)
	adminHandler.SetAdminFinanceUC(adminFinanceUC)
	reimbursementHandler := handler.NewReimbursementHandler(adminFinanceUC)
	serviceUsecase.SetCategoryRepository(categoryRepo)
	if seeded, err := adminCMSUC.EnsureSeedCategories(context.Background()); err != nil {
		logger.Log.Error().Err(err).Msg("failed to seed service taxonomy")
	} else if seeded > 0 {
		logger.Log.Info().Int64("categories", seeded).Msg("seeded service taxonomy")
	}
	usecase.PromoteAdmins(context.Background(), userRepo, cfg.AdminEmails)

	authMiddleware := middleware.NewAuthMiddleware(cfg.JWTSecret, tokenRepo)
	// F1: Authenticate rejects suspended accounts; the directory lookup is
	// attached here so the gate sees live status.
	authMiddleware.SetUserRepository(userRepo)
	rateLimiter := middleware.NewRateLimiter(redisClient, 100, time.Minute)

	imageProcessor := imaging.NewProcessor()
	uploadUsecase := usecase.NewUploadUsecase(storageRepo, imageProcessor)
	uploadHandler := handler.NewUploadHandler(uploadUsecase)

	app := fiber.New(fiber.Config{
		AppName:      cfg.AppName,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		ErrorHandler: apierror.GlobalErrorHandler,
		BodyLimit:    50 * 1024 * 1024,
	})

	app.Use(requestid.New(requestid.Config{
		Generator: func() string {
			return fmt.Sprintf("req-%d", time.Now().UnixNano())
		},
	}))
	app.Use(func(c fiber.Ctx) error {
		requestID := c.GetRespHeader("X-Request-Id")
		requestctx.SetRequestID(c, requestID)
		return c.Next()
	})
	app.Use(recover.New())
	app.Use(helmet.New())
	app.Use(cors.New(cors.Config{
		AllowOrigins: []string{"*"},
		AllowMethods: []string{"GET", "POST", "PUT", "DELETE", "PATCH", "OPTIONS"},
		AllowHeaders: []string{"Origin", "Content-Type", "Accept", "Authorization"},
	}))
	app.Use(compress.New())
	app.Use(middleware.RequestLogger(logger.Log))
	app.Use(middleware.Timeout(30 * time.Second))
	registerLocalUploads(app, cfg)

	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	api := app.Group("/api/v1")
	api.Use(middleware.AuditLogger(logger.Log))

	api.Post("/auth/register", middleware.RequireModule(killSwitchUC, "registration"), authHandler.Register)
	api.Post("/auth/login", authHandler.Login)
	api.Post("/auth/refresh", authHandler.RefreshToken)
	api.Post("/auth/logout", authHandler.Logout)
	api.Post("/auth/introspect", introspectHandler.Introspect)
	// F3: service-to-service chat-access signal for the chat backend's
	// live-session enforcement. Shares the CHAT_SERVICE_TOKEN secret (the
	// same value the chat backend's own admin endpoint expects); user JWTs
	// are never accepted here.
	internalGroup := api.Group("/internal")
	internalGroup.Use(middleware.RequireServiceToken(cfg.ChatServiceToken))
	internalGroup.Get("/chat-access/:id", handler.NewInternalHandler(userRepo).ChatAccess)
	api.Get("/users/:id", userHandler.GetPublicProfile)
	api.Get("/users/:id/stats", userHandler.GetStats)
	api.Get("/users/:id/reviews", reviewHandler.GetReceivedReviews)

	api.Get("/services", serviceHandler.List)
	api.Get("/services/:id", serviceHandler.Get)
	api.Get("/banners", cmsHandler.ListBanners)
	api.Get("/categories", cmsHandler.ListCategories)
	api.Get("/services/:id/reviews", reviewHandler.GetServiceReviews)
	api.Get("/providers/:id/reviews", reviewHandler.GetProviderReviews)
	api.Get("/providers/:id/availability", bookingHandler.GetAvailableSlots)

	protected := api.Group("")
	protected.Use(authMiddleware.Authenticate())
	protected.Use(rateLimiter.Limit())

	protected.Get("/me", profileHandler.GetProfile)
	protected.Patch("/me", profileHandler.UpdateProfile)

	protected.Post("/services", serviceHandler.Create)
	protected.Patch("/services/:id", serviceHandler.Update)
	protected.Delete("/services/:id", serviceHandler.SetStatus)
	protected.Get("/my-services", serviceHandler.GetMyServices)

	protected.Post("/bookings", middleware.RequireModule(killSwitchUC, "bookings"), bookingHandler.Create)
	protected.Post("/bids", bidHandler.Create)
	protected.Get("/bids/my", bidHandler.GetMyBids)
	protected.Get("/services/:id/bids", bidHandler.GetByService)
	protected.Patch("/bids/:id/accept", bidHandler.Accept)
	protected.Patch("/bids/:id/reject", bidHandler.Reject)
	protected.Get("/bookings/:id", bookingHandler.Get)
	protected.Get("/my-bookings", bookingHandler.GetMyBookings)
	protected.Patch("/bookings/:id/accept", bookingHandler.Accept)
	protected.Patch("/bookings/:id/reject", bookingHandler.Reject)
	protected.Patch("/bookings/:id/confirm", bookingHandler.Confirm)
	protected.Patch("/bookings/:id/complete", bookingHandler.Complete)
	protected.Patch("/bookings/:id/cancel", bookingHandler.Cancel)
	protected.Post("/reviews", reviewHandler.Create)
	protected.Post("/upload", uploadHandler.Upload)
	protected.Get("/notifications", notificationHandler.List)
	protected.Get("/notifications/stream", notificationHandler.Stream)
	protected.Patch("/notifications/:id/read", notificationHandler.MarkRead)
	protected.Post("/reimbursements", reimbursementHandler.File)

	admin := api.Group("/admin")
	admin.Use(authMiddleware.Authenticate())
	admin.Use(rateLimiter.Limit())
	admin.Use(middleware.RequireAdmin(profileUsecase))
	admin.Use(middleware.AdminAuditLogger(auditRepo, logger.Log))
	admin.Get("/cancellations", adminHandler.ListCancellations)
	admin.Get("/notifications", adminHandler.ListNotifications)
	admin.Get("/stats", adminHandler.GetStats)
	admin.Get("/bookings", adminHandler.ListBookings)
	admin.Get("/bookings/:id", adminHandler.GetBooking)
	admin.Patch("/bookings/:id/cancel", adminHandler.CancelBooking)
	admin.Get("/audit-logs", adminHandler.ListAuditLogs)
	admin.Get("/users", adminHandler.ListUsers)
	admin.Get("/users/:id", adminHandler.GetUser)
	admin.Patch("/users/:id/suspend", adminHandler.SuspendUser)
	admin.Patch("/users/:id/restore", adminHandler.RestoreUser)
	admin.Patch("/users/:id/role", adminHandler.SetUserRole)
	admin.Patch("/users/:id/chat-block", adminHandler.SetChatBlock)
	admin.Get("/chat/conversations", adminHandler.ListChatConversations)
	admin.Get("/services", adminHandler.ListServices)
	admin.Get("/services/:id", adminHandler.GetService)
	admin.Patch("/services/:id/status", adminHandler.SetServiceStatus)
	admin.Get("/reviews", adminHandler.ListReviews)
	admin.Patch("/reviews/:id/visibility", adminHandler.SetReviewVisibility)
	admin.Get("/reports", adminHandler.ListReports)
	admin.Patch("/reports/:id", adminHandler.ResolveReport)
	admin.Get("/broadcasts", adminHandler.ListBroadcasts)
	admin.Get("/broadcasts/estimate", adminHandler.EstimateBroadcastRecipients)
	admin.Post("/broadcasts", middleware.RequireModule(killSwitchUC, "broadcasts"), adminHandler.CreateBroadcast)
	admin.Post("/broadcasts/:id/cancel", adminHandler.CancelBroadcast)
	admin.Get("/banners", adminHandler.ListBanners)
	admin.Post("/banners", adminHandler.CreateBanner)
	admin.Patch("/banners/:id", adminHandler.UpdateBanner)
	admin.Delete("/banners/:id", adminHandler.DeleteBanner)
	admin.Get("/categories", adminHandler.ListCategories)
	admin.Post("/categories", adminHandler.CreateCategory)
	admin.Patch("/categories/:id", adminHandler.UpdateCategory)
	admin.Delete("/categories/:id", adminHandler.DeleteCategory)
	admin.Get("/finance/overview", adminHandler.GetFinanceOverview)
	admin.Get("/finance/summary", adminHandler.GetFinanceSummary)
	admin.Get("/transactions", adminHandler.ListTransactions)
	admin.Get("/payouts", adminHandler.ListPayouts)
	admin.Post("/payouts/mark", adminHandler.MarkPayoutPaid)
	admin.Get("/reimbursements", adminHandler.ListReimbursements)
	admin.Patch("/reimbursements/:id", adminHandler.DecideReimbursement)
	admin.Get("/kill-switches", adminHandler.ListKillSwitches)
	admin.Patch("/kill-switches/:module", adminHandler.SetKillSwitch)

	logger.Log.Info().Int("port", cfg.Port).Msg("server starting")

	// Ticket 9: scheduled-broadcast due-poller (60s ticker, in-app
	// notifications only). Runs once at startup plus every interval;
	// stops via context on graceful shutdown.
	broadcastPollerCtx, stopBroadcastPoller := context.WithCancel(context.Background())
	usecase.StartBroadcastDuePoller(broadcastPollerCtx, adminBroadcastUC, 60*time.Second)

	go func() {
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			logger.Log.Fatal().Err(err).Msg("server failed to start")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info().Msg("shutting down server")

	stopBroadcastPoller()

	if err := app.Shutdown(); err != nil {
		logger.Log.Error().Err(err).Msg("error during shutdown")
	}

	sqlDB, err := db.DB()
	if err == nil {
		sqlDB.Close()
	}

	redisClient.Close()

	logger.Log.Info().Msg("server stopped")
}

func registerLocalUploads(app *fiber.App, cfg *config.Config) {
	if cfg.StorageDriver != "local" {
		return
	}

	publicURL, err := localrepo.ParsePublicURL(cfg.StorageLocalURL)
	if err != nil {
		return
	}

	// F2: media URLs are stored absolute, so the storefront embeds them from
	// a different origin (in dev, localhost:3000 directly). Set the embed
	// policy on the prefix itself, before the static handler, so it also
	// covers the 404s/error responses that the handler's ModifyResponse never
	// sees — otherwise Chrome blocks those images with
	// ERR_BLOCKED_BY_RESPONSE.NotSameOrigin instead of showing a broken image.
	app.Use(publicURL.Path, func(c fiber.Ctx) error {
		c.Set("Cross-Origin-Resource-Policy", "cross-origin")
		return c.Next()
	})

	app.Use(publicURL.Path, staticmiddleware.New(cfg.StorageLocalPath, staticmiddleware.Config{
		Browse:        false,
		CacheDuration: -1,
		NotFoundHandler: func(c fiber.Ctx) error {
			return c.SendStatus(fiber.StatusNotFound)
		},
	}))
}
