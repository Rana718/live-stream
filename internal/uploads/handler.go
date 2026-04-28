// Package uploads exposes a generic admin/instructor file upload endpoint.
//
// Why a separate package from materials: study materials are first-class
// rows in the DB (with title, topic, language, free flag etc). Branding
// logos, course thumbnails, banner images, lecture videos and rich-text
// editor inline images don't need that — they only need a public URL.
// This package proxies the bytes through Fiber to MinIO, returns the
// canonical URL, and lets the caller persist that URL on whatever row
// they own.
package uploads

import (
	"context"
	"fmt"
	"path/filepath"
	"strings"
	"time"

	"live-platform/internal/config"

	"github.com/gofiber/fiber/v3"
	"github.com/minio/minio-go/v7"
)

type Handler struct {
	minio     *minio.Client
	cfg       *config.MinIOConfig
	publicURL string // optional CDN/base URL for served objects
}

// NewHandler wires a generic uploader. publicBase, when non-empty, is the
// base URL prepended to returned object keys (e.g. https://cdn.example.com).
// When empty, the handler returns presigned GET URLs valid for 7 days so
// uploads work in dev without a CDN in front of MinIO.
func NewHandler(mc *minio.Client, cfg *config.MinIOConfig, publicBase string) *Handler {
	return &Handler{minio: mc, cfg: cfg, publicURL: strings.TrimRight(publicBase, "/")}
}

// allowed folders → bucket mapping. We reuse the existing buckets the
// platform already provisions (recordings, materials) instead of forcing
// ops to create new ones. "video" goes to recordings, everything else
// (image/logo/thumbnail/banner/post) to materials.
func (h *Handler) bucketFor(kind string) string {
	switch kind {
	case "video", "lecture":
		return h.cfg.Bucket // recordings
	default:
		return h.cfg.MaterialsBucket
	}
}

// Upload godoc
// @Summary Upload an asset (image/video/document) to object storage
// @Description Returns a public URL to the stored object. Caller stores the URL on its own row.
// @Tags uploads
// @Accept multipart/form-data
// @Security BearerAuth
// @Param file formData file true "File"
// @Param kind formData string false "image|video|document|logo|thumbnail|banner|post (default: image)"
// @Param folder formData string false "Optional sub-folder under the bucket"
// @Success 200 {object} map[string]any
// @Router /uploads [post]
func (h *Handler) Upload(c fiber.Ctx) error {
	file, err := c.FormFile("file")
	if err != nil {
		return c.Status(fiber.StatusBadRequest).JSON(fiber.Map{"error": "file required"})
	}
	kind := strings.ToLower(strings.TrimSpace(c.FormValue("kind")))
	if kind == "" {
		kind = "image"
	}
	folder := strings.Trim(strings.TrimSpace(c.FormValue("folder")), "/")
	if folder == "" {
		folder = kind
	}

	bucket := h.bucketFor(kind)

	f, err := file.Open()
	if err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": "cannot open file"})
	}
	defer f.Close()

	ext := strings.ToLower(filepath.Ext(file.Filename))
	objectName := fmt.Sprintf("%s/%d-%s%s",
		folder,
		time.Now().UnixNano(),
		safeStem(file.Filename),
		ext,
	)

	contentType := file.Header.Get("Content-Type")
	if contentType == "" {
		contentType = "application/octet-stream"
	}

	if _, err := h.minio.PutObject(c.Context(), bucket, objectName, f, file.Size, minio.PutObjectOptions{
		ContentType: contentType,
	}); err != nil {
		return c.Status(fiber.StatusInternalServerError).JSON(fiber.Map{"error": err.Error()})
	}

	url := h.urlFor(c.Context(), bucket, objectName)
	return c.JSON(fiber.Map{
		"url":          url,
		"key":          objectName,
		"bucket":       bucket,
		"size":         file.Size,
		"content_type": contentType,
	})
}

// urlFor returns either the CDN/public URL or a long-TTL presigned GET URL
// so dev environments without a CDN still serve the asset.
func (h *Handler) urlFor(ctx context.Context, bucket, object string) string {
	if h.publicURL != "" {
		return fmt.Sprintf("%s/%s/%s", h.publicURL, bucket, object)
	}
	u, err := h.minio.PresignedGetObject(ctx, bucket, object, 7*24*time.Hour, nil)
	if err != nil {
		return ""
	}
	return u.String()
}

// safeStem strips the extension and replaces non-alphanumerics with `-`
// so the resulting object key is URL-safe and predictable.
func safeStem(name string) string {
	stem := strings.TrimSuffix(name, filepath.Ext(name))
	var b strings.Builder
	for _, r := range stem {
		switch {
		case r >= 'a' && r <= 'z', r >= 'A' && r <= 'Z', r >= '0' && r <= '9', r == '-', r == '_':
			b.WriteRune(r)
		default:
			b.WriteRune('-')
		}
	}
	out := strings.Trim(b.String(), "-")
	if out == "" {
		out = "file"
	}
	if len(out) > 60 {
		out = out[:60]
	}
	return out
}
