// Package appbuilds is the per-tenant white-label app build pipeline.
// Workflow:
//   1. super_admin POSTs /admin/platform/tenants/:id/build → row in
//      app_builds with status='queued', Codemagic dispatch attempted.
//   2. Codemagic runs the build using the tenant's branding JSON +
//      package_id, posts back to /webhooks/codemagic with status updates.
//   3. On success the build_url + play_url get patched into the row.
//
// If CODEMAGIC_WORKFLOW_ID is empty (dev / first-tenant manual mode), the
// row is still created so a human operator can pick it up out-of-band and
// patch status via the existing super_admin patch handler.
package appbuilds

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"
	"io"
	"log/slog"
	"net/http"
	"strings"
	"time"

	"live-platform/internal/config"
	"live-platform/internal/database/db"
	"live-platform/internal/utils"

	"github.com/google/uuid"
	"github.com/jackc/pgx/v5/pgtype"
	"github.com/jackc/pgx/v5/pgxpool"
)

type Service struct {
	q    *db.Queries
	cfg  config.CodemagicConfig
	http *http.Client
	log  *slog.Logger
}

func NewService(pool *pgxpool.Pool, cfg config.CodemagicConfig, log *slog.Logger) *Service {
	timeout := time.Duration(cfg.TimeoutSec) * time.Second
	if timeout == 0 {
		timeout = 10 * time.Second
	}
	return &Service{
		q:    db.New(pool),
		cfg:  cfg,
		http: &http.Client{Timeout: timeout},
		log:  log,
	}
}

type TriggerInput struct {
	Platform     string `json:"platform" validate:"required,oneof=android ios"`
	PackageID    string `json:"package_id" validate:"required"`
	VersionName  string `json:"version_name"`
}

// Trigger creates a queued build row and (if Codemagic is configured)
// dispatches the build. The DB row is the source of truth — we record it
// even if the dispatch fails so support can re-trigger from the UI later.
func (s *Service) Trigger(ctx context.Context, tenantID uuid.UUID, in TriggerInput) (*db.AppBuild, error) {
	if in.VersionName == "" {
		in.VersionName = time.Now().Format("2006.01.02")
	}
	row, err := s.q.CreateAppBuild(ctx, db.CreateAppBuildParams{
		TenantID:    utils.UUIDToPg(tenantID),
		Platform:    in.Platform,
		PackageID:   pgtype.Text{String: in.PackageID, Valid: true},
		VersionName: pgtype.Text{String: in.VersionName, Valid: true},
	})
	if err != nil {
		return nil, err
	}

	if s.cfg.APIToken == "" || s.cfg.WorkflowID == "" || s.cfg.AppID == "" {
		// Manual mode: the row exists but no automated dispatch.
		s.log.Info("appbuild queued (manual mode — Codemagic not configured)",
			slog.String("build_id", uuid.UUID(row.ID.Bytes).String()))
		return &row, nil
	}

	if err := s.dispatchToCodemagic(ctx, row, tenantID, in); err != nil {
		// Patch the row to failed but still return success — the UI can
		// show the error_log and a Retry button.
		_, _ = s.q.SetAppBuildStatus(ctx, db.SetAppBuildStatusParams{
			ID:       row.ID,
			Status:   "failed",
			Column3:  "",
			Column4:  "",
			Column5:  err.Error(),
		})
		s.log.Error("codemagic dispatch failed",
			slog.String("build_id", uuid.UUID(row.ID.Bytes).String()),
			slog.String("err", err.Error()))
	}
	return &row, nil
}

type codemagicReq struct {
	AppID            string            `json:"appId"`
	WorkflowID       string            `json:"workflowId"`
	Branch           string            `json:"branch,omitempty"`
	EnvironmentVars  map[string]string `json:"environment,omitempty"`
}

func (s *Service) dispatchToCodemagic(ctx context.Context, row db.AppBuild, tenantID uuid.UUID, in TriggerInput) error {
	envs := map[string]string{
		"TENANT_ID":     tenantID.String(),
		"BUILD_ID":      uuid.UUID(row.ID.Bytes).String(),
		"PACKAGE_ID":    in.PackageID,
		"VERSION_NAME":  in.VersionName,
		"PLATFORM":      in.Platform,
	}
	body, _ := json.Marshal(codemagicReq{
		AppID:           s.cfg.AppID,
		WorkflowID:      s.cfg.WorkflowID,
		Branch:          "main",
		EnvironmentVars: envs,
	})

	url := strings.TrimRight(s.cfg.BaseURL, "/") + "/builds"
	req, err := http.NewRequestWithContext(ctx, http.MethodPost, url, bytes.NewReader(body))
	if err != nil {
		return err
	}
	req.Header.Set("Content-Type", "application/json")
	req.Header.Set("x-auth-token", s.cfg.APIToken)

	resp, err := s.http.Do(req)
	if err != nil {
		return err
	}
	defer resp.Body.Close()

	raw, _ := io.ReadAll(resp.Body)
	if resp.StatusCode >= 400 {
		return fmt.Errorf("codemagic %d: %s", resp.StatusCode, string(raw))
	}
	s.log.Info("codemagic build dispatched",
		slog.String("build_id", uuid.UUID(row.ID.Bytes).String()))
	return nil
}

func (s *Service) List(ctx context.Context, status string, limit, offset int32) ([]db.ListAppBuildsRow, error) {
	return s.q.ListAppBuilds(ctx, db.ListAppBuildsParams{
		Column1: status,
		Limit:   limit,
		Offset:  offset,
	})
}

// SetStatus is invoked by the Codemagic webhook (or manually by support).
type SetStatusInput struct {
	Status   string `json:"status" validate:"required,oneof=queued building published failed"`
	BuildURL string `json:"build_url"`
	PlayURL  string `json:"play_url"`
	ErrorLog string `json:"error_log"`
}

func (s *Service) SetStatus(ctx context.Context, id uuid.UUID, in SetStatusInput) (*db.AppBuild, error) {
	row, err := s.q.SetAppBuildStatus(ctx, db.SetAppBuildStatusParams{
		ID:      utils.UUIDToPg(id),
		Status:  in.Status,
		Column3: in.BuildURL,
		Column4: in.PlayURL,
		Column5: in.ErrorLog,
	})
	if err != nil {
		return nil, err
	}
	return &row, nil
}
