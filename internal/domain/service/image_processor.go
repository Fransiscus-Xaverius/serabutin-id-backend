package service

type VariantConfig struct {
	Name      string
	MaxWidth  int
	MaxHeight int
}

type ProcessOptions struct {
	Variants []VariantConfig
	Quality  int
}

type ProcessedImage struct {
	Name        string
	Data        []byte
	ContentType string
	Width       int
	Height      int
}

type ImageProcessor interface {
	Process(data []byte, opts ProcessOptions) ([]ProcessedImage, error)
}
