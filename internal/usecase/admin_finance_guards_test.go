package usecase

import (
	"context"
	"testing"

	"github.com/stretchr/testify/assert"
	"github.com/stretchr/testify/require"

	apperror "github.com/serabutin/serabutin-id-backend/internal/domain/error"
)

// F11: a second filing is rejected while a pending claim exists for the
// same booking. F12: phantom payout marks (no earnings in period) are
// rejected.

func TestAdminFinance_FileReimbursement_DuplicatePending_Conflict(t *testing.T) {
	uc, _, _ := newFinanceTestUC()

	first, err := uc.FileReimbursement(context.Background(), 7, 42, 50000, "biaya tol", "")
	require.NoError(t, err)
	require.NotNil(t, first)

	second, err := uc.FileReimbursement(context.Background(), 7, 42, 60000, "biaya tol susulan", "")

	require.Error(t, err)
	assert.Nil(t, second)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "DUPLICATE_CLAIM", domainErr.Code)
	assert.ErrorIs(t, domainErr.Err, apperror.ErrAlreadyExists)
}

func TestAdminFinance_FileReimbursement_AfterDecision_CanFileAgain(t *testing.T) {
	uc, _, _ := newFinanceTestUC()

	first, err := uc.FileReimbursement(context.Background(), 7, 42, 50000, "biaya tol", "")
	require.NoError(t, err)

	_, err = uc.DecideReimbursement(context.Background(), 1, first.ID, "rejected")
	require.NoError(t, err)

	second, err := uc.FileReimbursement(context.Background(), 7, 42, 60000, "biaya tol susulan", "")
	require.NoError(t, err)
	require.NotNil(t, second)
}

func TestAdminFinance_MarkPayoutPaid_NoEarnings_ValidationError(t *testing.T) {
	uc, _, _ := newFinanceTestUC()

	// Provider 8 has no computed earnings in 2026-10 (fake only pays 7).
	mark, err := uc.MarkPayoutPaid(context.Background(), 1, 8, "2026-10")

	require.Error(t, err)
	assert.Nil(t, mark)
	var domainErr *apperror.DomainError
	require.ErrorAs(t, err, &domainErr)
	assert.Equal(t, "VALIDATION_ERROR", domainErr.Code)
}
