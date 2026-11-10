package handler

import (
	"context"

	"github.com/gofiber/fiber/v3"

	"github.com/serabutin/serabutin-id-backend/internal/domain/entity"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/requestctx"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
	"github.com/serabutin/serabutin-id-backend/pkg/validator"
)

// profileRoleReader is the part of the profile usecase the payout handler
// needs to check the caller's role.
type profileRoleReader interface {
	GetProfile(ctx context.Context, userID uint) (*entity.User, error)
}

type PayoutHandler struct {
	payoutUsecase usecase.PayoutAccountService
	profiles      profileRoleReader
}

func NewPayoutHandler(payoutUsecase usecase.PayoutAccountService, profiles profileRoleReader) *PayoutHandler {
	return &PayoutHandler{payoutUsecase: payoutUsecase, profiles: profiles}
}

// requireProvider rejects anyone who is not a Mitra: only a provider gets
// paid, so only a provider may set a payout destination.
func (h *PayoutHandler) requireProvider(c fiber.Ctx) (uint, *apierror.APIError) {
	userID := requestctx.GetUserID(c)
	if userID == 0 {
		return 0, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
	}
	profile, err := h.profiles.GetProfile(c.Context(), userID)
	if err != nil || profile == nil {
		return 0, apierror.NewAPIError(fiber.StatusUnauthorized, "UNAUTHORIZED", "unauthorized")
	}
	if profile.Role != "provider" {
		return 0, apierror.NewAPIError(fiber.StatusForbidden, "FORBIDDEN", "only a mitra can manage a payout account")
	}
	return userID, nil
}

func (h *PayoutHandler) Upsert(c fiber.Ctx) error {
	userID, apiErr := h.requireProvider(c)
	if apiErr != nil {
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	var req dto.PayoutAccountRequest
	if err := c.Bind().Body(&req); err != nil {
		return apierror.GlobalErrorHandler(c, apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "invalid request body"))
	}
	if validationErrors := validator.Validate(req); validationErrors != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "VALIDATION_ERROR", "validation failed")
		apiErr.Details = validationErrors
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	account, err := h.payoutUsecase.Upsert(c.Context(), userID, req)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.Success(c, fiber.StatusOK, toPayoutAccountResponse(account))
}

func (h *PayoutHandler) Get(c fiber.Ctx) error {
	userID, apiErr := h.requireProvider(c)
	if apiErr != nil {
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	account, err := h.payoutUsecase.Get(c.Context(), userID)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}
	return response.Success(c, fiber.StatusOK, toPayoutAccountResponse(account))
}

// toPayoutAccountResponse is the only shape a payout account leaves the API
// in: never the account number, never the holder name.
func toPayoutAccountResponse(account *entity.PayoutAccount) dto.PayoutAccountResponse {
	return dto.PayoutAccountResponse{
		BankCode:     account.BankCode,
		BankName:     account.BankName,
		AccountLast4: account.AccountLast4,
		PhoneLast4:   account.PhoneLast4,
		Status:       account.Status,
		NameMismatch: account.NameMismatch,
		UpdatedAt:    account.UpdatedAt.Format("2006-01-02T15:04:05Z"),
	}
}
