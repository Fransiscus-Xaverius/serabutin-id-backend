package main

import (
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

	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
	redisrepo "github.com/serabutin/serabutin-id-backend/internal/repository/redis"
	gormrepo "github.com/serabutin/serabutin-id-backend/internal/repository/gorm"
	"github.com/serabutin/serabutin-id-backend/internal/handler"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/config"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/database"
	"github.com/serabutin/serabutin-id-backend/internal/infrastructure/logger"
	infraredis "github.com/serabutin/serabutin-id-backend/internal/infrastructure/redis"
	"github.com/serabutin/serabutin-id-backend/internal/middleware"
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

	if err := db.AutoMigrate(&model.UserModel{}); err != nil {
		logger.Log.Fatal().Err(err).Msg("failed to run auto migration")
	}
	logger.Log.Info().Msg("database migrated successfully")

	userRepo := gormrepo.NewUserRepository(db)
	tokenRepo := redisrepo.NewTokenRepository(redisClient)
	tokenService := service.NewTokenService(cfg.JWTSecret, cfg.JWTRefreshSecret, cfg.AccessTokenDuration, cfg.RefreshTokenDuration)

	authUsecase := usecase.NewAuthUsecase(userRepo, tokenRepo, tokenService, cfg.BcryptCost)
	authHandler := handler.NewAuthHandler(authUsecase)
	authMiddleware := middleware.NewAuthMiddleware(cfg.JWTSecret, tokenRepo)
	rateLimiter := middleware.NewRateLimiter(redisClient, 100, time.Minute)

	app := fiber.New(fiber.Config{
		AppName:      cfg.AppName,
		ReadTimeout:  10 * time.Second,
		WriteTimeout: 30 * time.Second,
		IdleTimeout:  60 * time.Second,
		ErrorHandler: apierror.GlobalErrorHandler,
		BodyLimit:    1 * 1024 * 1024,
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

	// Health check
	app.Get("/health", func(c fiber.Ctx) error {
		return c.JSON(fiber.Map{
			"status": "ok",
			"time":   time.Now().UTC().Format(time.RFC3339),
		})
	})

	// Auth routes (public)
	api := app.Group("/api/v1")

	api.Post("/auth/register", authHandler.Register)
	api.Post("/auth/login", authHandler.Login)
	api.Post("/auth/refresh", authHandler.RefreshToken)
	api.Post("/auth/logout", authHandler.Logout)

	// Protected routes example
	protected := api.Group("")
	protected.Use(authMiddleware.Authenticate())
	protected.Use(rateLimiter.Limit())

	protected.Get("/me", func(c fiber.Ctx) error {
		userID := requestctx.GetUserID(c)
		email := requestctx.GetUserEmail(c)
		return c.JSON(fiber.Map{
			"user_id": userID,
			"email":   email,
		})
	})

	logger.Log.Info().Int("port", cfg.Port).Msg("server starting")

	go func() {
		if err := app.Listen(fmt.Sprintf(":%d", cfg.Port)); err != nil {
			logger.Log.Fatal().Err(err).Msg("server failed to start")
		}
	}()

	quit := make(chan os.Signal, 1)
	signal.Notify(quit, syscall.SIGINT, syscall.SIGTERM)
	<-quit

	logger.Log.Info().Msg("shutting down server")

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
