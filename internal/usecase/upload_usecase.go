package usecase

import (
	"context"
	"fmt"
	"log"

	"github.com/google/uuid"
	"github.com/serabutin/serabutin-id-backend/internal/domain/repository"
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
	"github.com/serabutin/serabutin-id-backend/internal/dto"
)

type FileData struct {
	Filename    string
	Data        []byte
	ContentType string
}

type UploadUsecase struct {
	storage   repository.StorageRepository
	processor service.ImageProcessor
}

func NewUploadUsecase(storage repository.StorageRepository, processor service.ImageProcessor) *UploadUsecase {
	return &UploadUsecase{
		storage:   storage,
		processor: processor,
	}
}

func (uc *UploadUsecase) Upload(ctx context.Context, files []FileData) ([]dto.UploadedFile, error) {
	var results []dto.UploadedFile

	for _, file := range files {
		key := uuid.New().String()

		opts := service.ProcessOptions{
			Variants: []service.VariantConfig{
				{Name: "thumbnail", MaxWidth: 200, MaxHeight: 200},
				{Name: "medium", MaxWidth: 800, MaxHeight: 800},
				{Name: "original"},
			},
			Quality: 85,
		}

		processed, err := uc.processor.Process(file.Data, opts)
		if err != nil {
			log.Printf("[UploadUsecase.Upload] process image error: %v", err)
			return nil, fmt.Errorf("failed to process image: %w", err)
		}

		var uploaded dto.UploadedFile
		uploaded.Key = key

		for _, img := range processed {
			variantKey := fmt.Sprintf("%s/%s.jpg", key, img.Name)
			url, err := uc.storage.Upload(ctx, variantKey, img.Data, img.ContentType)
			if err != nil {
				log.Printf("[UploadUsecase.Upload] upload variant %s error: %v", img.Name, err)
				return nil, fmt.Errorf("failed to upload variant %s: %w", img.Name, err)
			}

			switch img.Name {
			case "thumbnail":
				uploaded.ThumbnailURL = url
			case "medium":
				uploaded.MediumURL = url
			case "original":
				uploaded.OriginalURL = url
			}
		}

		results = append(results, uploaded)
	}

	return results, nil
}
