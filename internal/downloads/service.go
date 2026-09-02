// Package downloads — schema-v2. video_variants → video_assets +
// video_renditions. Offline-download DRM tokens (download_tokens table) are
// deferred post-launch per the plan, so IssueToken/Resolve return a clear
// "not available" until that lands.
package downloads

import (
	"context"
	"errors"

	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
	"github.com/minio/minio-go/v7"
)

type Service struct {
	q       *db.Queries
	minio   *minio.Client
	bucket  string
	baseURL string
}

func NewService(pool *pgxpool.Pool, mc *minio.Client, bucket, baseURL string) *Service {
	return &Service{q: db.New(pool), minio: mc, bucket: bucket, baseURL: baseURL}
}

var errDeferred = errors.New("offline downloads are not available in this build")

type CreateVariantRequest struct {
	VideoAssetID uuid.UUID `json:"video_asset_id" validate:"required"`
	Height       int32     `json:"height" validate:"required"`
	FileKey      string    `json:"file_key" validate:"required"`
	FilePath     string    `json:"file_path"`
	FileSize     int64     `json:"file_size"`
	BitrateKbps  int32     `json:"bitrate_kbps"`
	Codec        string    `json:"codec"`
}

type Rendition struct {
	Height      int32  `json:"height"`
	Quality     string `json:"quality"`
	BitrateKbps int32  `json:"bitrate_kbps"`
	Codec       string `json:"codec"`
	FileKey     string `json:"file_key"`
	FilePath    string `json:"file_path"`
	FileSize    int64  `json:"file_size"`
}

func (s *Service) CreateVariant(ctx context.Context, tenantID uuid.UUID, req CreateVariantRequest) (Rendition, error) {
	if req.Codec == "" {
		req.Codec = "h264"
	}
	key := req.FileKey
	if key == "" {
		key = req.FilePath
	}
	if err := s.q.AddVideoRendition(ctx, db.AddVideoRenditionParams{
		TenantID:     utils.UUIDToPg(tenantID),
		VideoAssetID: utils.UUIDToPg(req.VideoAssetID),
		Height:       req.Height,
		FileKey:      key,
		BitrateKbps:  pgtype.Int4{Int32: req.BitrateKbps, Valid: req.BitrateKbps > 0},
		Codec:        pgtype.Text{String: req.Codec, Valid: true},
		FileSize:     pgtype.Int8{Int64: req.FileSize, Valid: req.FileSize > 0},
	}); err != nil {
		return Rendition{}, err
	}
	return Rendition{Height: req.Height, Quality: qualityLabel(req.Height), BitrateKbps: req.BitrateKbps,
		Codec: req.Codec, FileKey: key, FilePath: key, FileSize: req.FileSize}, nil
}

func qualityLabel(h int32) string {
	if h <= 0 {
		return ""
	}
	return itoa(h) + "p"
}
func itoa(n int32) string {
	if n == 0 {
		return "0"
	}
	neg := n < 0
	if neg {
		n = -n
	}
	var b [12]byte
	i := len(b)
	for n > 0 {
		i--
		b[i] = byte('0' + n%10)
		n /= 10
	}
	if neg {
		i--
		b[i] = '-'
	}
	return string(b[i:])
}

// ListVariantsForLecture — legacy name. In v2 renditions key on
// video_asset_id, so the :lecture_id param is treated as an asset id.
func (s *Service) ListVariantsForLecture(ctx context.Context, videoAssetID uuid.UUID) ([]Rendition, error) {
	rows, err := s.q.ListVideoRenditions(ctx, utils.UUIDToPg(videoAssetID))
	if err != nil {
		return nil, err
	}
	out := make([]Rendition, 0, len(rows))
	for _, r := range rows {
		out = append(out, Rendition{
			Height: r.Height, Quality: qualityLabel(r.Height),
			BitrateKbps: r.BitrateKbps, Codec: r.Codec,
			FileKey: r.FileKey, FilePath: r.FileKey, FileSize: r.FileSize,
		})
	}
	return out, nil
}

type TokenRequest struct {
	ResourceType string    `json:"resource_type"`
	ResourceID   uuid.UUID `json:"resource_id"`
	TTLSeconds   int32     `json:"ttl_seconds"`
}

func (s *Service) IssueToken(ctx context.Context, userID uuid.UUID, req TokenRequest) (any, error) {
	return nil, errDeferred
}

func (s *Service) Resolve(ctx context.Context, token string) (string, error) {
	return "", errDeferred
}
