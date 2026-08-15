package web

import (
	"errors"

	"github.com/Antonio7098/ultraplan-go/internal/app"
)

func validateArtifact(result app.WebArtifactPreview) error {
	if result.MediaType != "text/markdown" && result.MediaType != "application/json" {
		return errors.New("unsupported artifact media type")
	}
	if result.ReturnedBytes < 0 || result.ReturnedBytes > app.WebPreviewByteLimit ||
		len([]byte(result.Content)) != result.ReturnedBytes || result.SizeBytes < int64(result.ReturnedBytes) {
		return errors.New("invalid artifact size contract")
	}
	if result.Truncated != (result.SizeBytes > int64(result.ReturnedBytes)) {
		return errors.New("invalid artifact truncation contract")
	}
	return nil
}
