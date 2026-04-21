package storage

import (
	"context"
	"fmt"
	"io"
	"live-platform/internal/config"
	"time"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
)

type MinIOClient struct {
	client *minio.Client
	bucket string
}

func NewMinIOClient(cfg *config.MinIOConfig) (*MinIOClient, error) {
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		return nil, fmt.Errorf("failed to create minio client: %w", err)
	}

	ctx := context.Background()
	exists, err := client.BucketExists(ctx, cfg.Bucket)
	if err != nil {
		return nil, fmt.Errorf("failed to check bucket: %w", err)
	}

	if !exists {
		err = client.MakeBucket(ctx, cfg.Bucket, minio.MakeBucketOptions{})
		if err != nil {
			return nil, fmt.Errorf("failed to create bucket: %w", err)
		}
	}

	return &MinIOClient{
		client: client,
		bucket: cfg.Bucket,
	}, nil
}

func (m *MinIOClient) UploadFile(ctx context.Context, objectName string, reader io.Reader, size int64, contentType string) error {
	_, err := m.client.PutObject(ctx, m.bucket, objectName, reader, size, minio.PutObjectOptions{
		ContentType: contentType,
	})
	return err
}

func (m *MinIOClient) GetFileURL(objectName string) (string, error) {
	url, err := m.client.PresignedGetObject(context.Background(), m.bucket, objectName, time.Hour, nil)
	if err != nil {
		return "", err
	}
	return url.String(), nil
}

func (m *MinIOClient) DeleteFile(ctx context.Context, objectName string) error {
	return m.client.RemoveObject(ctx, m.bucket, objectName, minio.RemoveObjectOptions{})
}

// Raw returns the underlying minio client for modules that need direct access.
func (m *MinIOClient) Raw() *minio.Client {
	return m.client
}

// EnsureBucket creates a bucket if it does not already exist.
func (m *MinIOClient) EnsureBucket(ctx context.Context, name string) error {
	exists, err := m.client.BucketExists(ctx, name)
	if err != nil {
		return err
	}
	if !exists {
		return m.client.MakeBucket(ctx, name, minio.MakeBucketOptions{})
	}
	return nil
}
