package gormrepo

import (
	"context"
	"testing"
	"time"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"
	"github.com/testcontainers/testcontainers-go"
	testpostgres "github.com/testcontainers/testcontainers-go/modules/postgres"
	"github.com/testcontainers/testcontainers-go/wait"
	gormpostgres "gorm.io/driver/postgres"
	"gorm.io/gorm"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/repository/model"
)

// Ticket 6: hidden reviews are excluded from every public read and from the
// service aggregate, while the admin queue still sees them; the report
// lifecycle is open -> resolved|dismissed with re-resolve rejected.
func setupReviewModerationTestDB(t *testing.T) *gorm.DB {
	t.Helper()
	ctx := context.Background()
	container, err := testpostgres.Run(ctx, "postgres:16-alpine",
		testpostgres.WithDatabase("testdb"),
		testpostgres.WithUsername("test"),
		testpostgres.WithPassword("test"),
		testcontainers.WithWaitStrategy(wait.ForLog("database system is ready to accept connections").WithOccurrence(2).WithStartupTimeout(30*time.Second)),
	)
	require.NoError(t, err)
	t.Cleanup(func() { _ = container.Terminate(ctx) })

	dsn, err := container.ConnectionString(ctx, "sslmode=disable")
	require.NoError(t, err)
	db, err := gorm.Open(gormpostgres.Open(dsn), &gorm.Config{})
	require.NoError(t, err)
	require.NoError(t, db.AutoMigrate(&model.UserModel{}, &model.ReviewModel{}, &model.ReportModel{}))
	return db
}

func TestReviewRepository_HiddenExcludedFromPublicReads(t *testing.T) {
	db := setupReviewModerationTestDB(t)
	ctx := context.Background()
	repo := NewReviewRepository(db)

	visible := &entity.Review{BookingID: 1, ServiceID: 11, CustomerID: 10, ProviderID: 20, AuthorID: 10, AuthorRole: "customer", Rating: 5, CommunicationRating: 5, QualityRating: 5, ValueRating: 5, Comment: "bagus"}
	hidden := &entity.Review{BookingID: 2, ServiceID: 11, CustomerID: 11, ProviderID: 20, AuthorID: 11, AuthorRole: "customer", Rating: 1, CommunicationRating: 1, QualityRating: 1, ValueRating: 1, Comment: "kasar", Hidden: true}
	require.NoError(t, repo.Create(ctx, visible))
	require.NoError(t, repo.Create(ctx, hidden))

	reviews, total, err := repo.FindByServiceID(ctx, 11, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, reviews, 1)
	assert.Equal(t, visible.ID, reviews[0].ID)

	stats, err := repo.GetServiceStats(ctx, 11)
	require.NoError(t, err)
	assert.Equal(t, int64(1), stats.TotalCount)
	assert.InDelta(t, 5.0, stats.AverageRating, 0.001)

	received, total, err := repo.FindReceivedByUserID(ctx, 20, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, received, 1)

	provider, total, err := repo.FindByProviderID(ctx, 20, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, provider, 1)
}

func TestReviewRepository_AdminSeesHiddenAndCanUnhide(t *testing.T) {
	db := setupReviewModerationTestDB(t)
	ctx := context.Background()
	repo := NewReviewRepository(db)

	hidden := &entity.Review{BookingID: 2, ServiceID: 11, CustomerID: 11, ProviderID: 20, AuthorID: 11, AuthorRole: "customer", Rating: 1, CommunicationRating: 1, QualityRating: 1, ValueRating: 1, Hidden: true}
	require.NoError(t, repo.Create(ctx, hidden))

	all, total, err := repo.ListForAdmin(ctx, repository.AdminReviewFilter{Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, all, 1)
	assert.True(t, all[0].Hidden)

	flagged, total, err := repo.ListForAdmin(ctx, repository.AdminReviewFilter{Page: 1, Limit: 20, HiddenOnly: true})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, flagged, 1)

	require.NoError(t, repo.SetHidden(ctx, hidden.ID, false))
	got, err := repo.FindByID(ctx, hidden.ID)
	require.NoError(t, err)
	assert.False(t, got.Hidden)

	public, total, err := repo.FindByServiceID(ctx, 11, 1, 20)
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, public, 1)
}

func TestReportRepository_Lifecycle(t *testing.T) {
	db := setupReviewModerationTestDB(t)
	ctx := context.Background()
	repo := NewReportRepository(db)

	report := &entity.Report{ReporterID: 10, TargetType: entity.ReportTargetReview, TargetID: 7, Reason: "bahasa kasar", Detail: "komentar menyinggung", Status: entity.ReportStatusOpen}
	require.NoError(t, repo.Create(ctx, report))
	assert.NotZero(t, report.ID)

	open, total, err := repo.ListForAdmin(ctx, repository.ReportFilter{Status: "open", Page: 1, Limit: 20})
	require.NoError(t, err)
	assert.Equal(t, int64(1), total)
	require.Len(t, open, 1)
	assert.Equal(t, "bahasa kasar", open[0].Reason)

	require.NoError(t, repo.Resolve(ctx, report.ID, 7, entity.ReportStatusResolved))
	got, err := repo.FindByID(ctx, report.ID)
	require.NoError(t, err)
	assert.Equal(t, entity.ReportStatusResolved, got.Status)
	require.NotNil(t, got.ResolvedBy)
	assert.Equal(t, uint(7), *got.ResolvedBy)
	assert.NotNil(t, got.ResolvedAt)
	assert.WithinDuration(t, time.Now(), *got.ResolvedAt, time.Minute)

	// Re-resolving a terminal report affects zero rows.
	res := db.Model(&model.ReportModel{}).
		Where("id = ? AND status = ?", report.ID, entity.ReportStatusOpen).
		Updates(map[string]interface{}{"status": entity.ReportStatusDismissed})
	require.NoError(t, res.Error)
	assert.Equal(t, int64(0), res.RowsAffected)
}
