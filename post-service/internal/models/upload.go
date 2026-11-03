package models

// UploadSignature is a short-lived, server-signed Cloudinary upload request.
// Clients PUT the file directly to Cloudinary so large payloads never touch the
// Go service.
type UploadSignature struct {
	CloudName string `json:"cloud_name"`
	APIKey    string `json:"api_key"`
	Timestamp int64  `json:"timestamp"`
	Signature string `json:"signature"`
	Folder    string `json:"folder"`
	PublicID  string `json:"public_id"`
	UploadURL string `json:"upload_url"`
	ExpiresAt int64  `json:"expires_at"`
}
