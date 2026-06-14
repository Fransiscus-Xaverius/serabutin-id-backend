package dto

type UploadResponse struct {
	Data []UploadedFile `json:"data"`
}

type UploadedFile struct {
	Key          string `json:"key"`
	OriginalURL  string `json:"original_url"`
	MediumURL    string `json:"medium_url"`
	ThumbnailURL string `json:"thumbnail_url"`
}
