package handler

import (
	"github.com/gofiber/fiber/v3"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
	"github.com/serabutin/serabutin-id-backend/internal/usecase"
	"github.com/serabutin/serabutin-id-backend/pkg/apierror"
	"github.com/serabutin/serabutin-id-backend/pkg/response"
)

const (
	maxFiles    = 8
	maxFileSize = 10 * 1024 * 1024
)

var allowedContentTypes = map[string]bool{
	"image/jpeg": true,
	"image/png":  true,
	"image/webp": true,
}

type UploadHandler struct {
	uploadUsecase *usecase.UploadUsecase
}

func NewUploadHandler(uploadUsecase *usecase.UploadUsecase) *UploadHandler {
	return &UploadHandler{
		uploadUsecase: uploadUsecase,
	}
}

func (h *UploadHandler) Upload(c fiber.Ctx) error {
	form, err := c.MultipartForm()
	if err != nil {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_REQUEST", "failed to parse multipart form")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	files := form.File["files"]
	if len(files) == 0 {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "NO_FILES", "no files provided")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	if len(files) > maxFiles {
		apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "TOO_MANY_FILES", "maximum 8 files allowed")
		return apierror.GlobalErrorHandler(c, apiErr)
	}

	var fileData []usecase.FileData
	for _, fh := range files {
		if fh.Size > maxFileSize {
			apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "FILE_TOO_LARGE", "file size exceeds 10MB limit")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		contentType := fh.Header.Get("Content-Type")
		if !allowedContentTypes[contentType] {
			apiErr := apierror.NewAPIError(fiber.StatusBadRequest, "INVALID_FILE_TYPE", "only JPEG, PNG, and WebP images are allowed")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		data, err := fh.Open()
		if err != nil {
			apiErr := apierror.NewAPIError(fiber.StatusInternalServerError, "FILE_READ_ERROR", "failed to read file")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		buf := make([]byte, fh.Size)
		_, err = data.Read(buf)
		data.Close()
		if err != nil && err.Error() != "EOF" {
			apiErr := apierror.NewAPIError(fiber.StatusInternalServerError, "FILE_READ_ERROR", "failed to read file")
			return apierror.GlobalErrorHandler(c, apiErr)
		}

		fileData = append(fileData, usecase.FileData{
			Filename:    fh.Filename,
			Data:        buf,
			ContentType: contentType,
		})
	}

	results, err := h.uploadUsecase.Upload(c.Context(), fileData)
	if err != nil {
		return apierror.GlobalErrorHandler(c, apierror.FromDomainError(err))
	}

	return response.Success(c, fiber.StatusCreated, dto.UploadResponse{Data: results})
}
