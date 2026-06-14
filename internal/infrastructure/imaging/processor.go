package imaging

import (
	"bytes"
	"fmt"
	"image"
	"image/jpeg"

	"github.com/disintegration/imaging"
	"github.com/serabutin/serabutin-id-backend/internal/domain/service"
)

type Processor struct{}

func NewProcessor() *Processor {
	return &Processor{}
}

func (p *Processor) Process(data []byte, opts service.ProcessOptions) ([]service.ProcessedImage, error) {
	src, _, err := image.Decode(bytes.NewReader(data))
	if err != nil {
		return nil, fmt.Errorf("failed to decode image: %w", err)
	}

	quality := opts.Quality
	if quality <= 0 {
		quality = 85
	}

	var results []service.ProcessedImage

	for _, variant := range opts.Variants {
		var processed image.Image

		if variant.MaxWidth > 0 || variant.MaxHeight > 0 {
			processed = imaging.Resize(src, variant.MaxWidth, variant.MaxHeight, imaging.Lanczos)
		} else {
			processed = src
		}

		var buf bytes.Buffer
		if err := jpeg.Encode(&buf, processed, &jpeg.Options{Quality: quality}); err != nil {
			return nil, fmt.Errorf("failed to encode variant %s: %w", variant.Name, err)
		}

		bounds := processed.Bounds()
		results = append(results, service.ProcessedImage{
			Name:        variant.Name,
			Data:        buf.Bytes(),
			ContentType: "image/jpeg",
			Width:       bounds.Dx(),
			Height:      bounds.Dy(),
		})
	}

	return results, nil
}
