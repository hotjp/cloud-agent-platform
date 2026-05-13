// Package storage implements L1-Storage layer: MinIO/S3-compatible cold storage.
// Uses minio-go SDK for object operations.
package storage

import (
	"bytes"
	"context"
	"errors"
	"fmt"
	"io"
	"strconv"
	"time"

	"github.com/cloud-agent-platform/cap/internal/config"

	"github.com/minio/minio-go/v7"
	"github.com/minio/minio-go/v7/pkg/credentials"
	"go.uber.org/zap"
)

// ErrL1ObjectNotFound is returned when an object does not exist in storage.
var ErrL1ObjectNotFound = errors.New("object not found in cold storage")

// DefaultPresignExpiry is the default expiry time for presigned URLs (1 hour).
const DefaultPresignExpiry = 1 * time.Hour

// MinIOStorage implements domain.ObjectStorage using MinIO/S3-compatible API.
type MinIOStorage struct {
	client    *minio.Client
	bucket    string
	logger    *zap.Logger
	opts      MinIOOptions
}

// MinIOOptions holds configuration options for MinIOStorage.
type MinIOOptions struct {
	// Bucket is the name of the bucket to use.
	Bucket string
	// UseSSL indicates whether to use SSL for connections.
	UseSSL bool
	// UploadExpiry is the expiry time for uploaded objects (for future use with lifecycle policies).
	// Currently, the cleanup worker handles expiry based on object metadata timestamps.
	UploadExpiry time.Duration
}

// NewMinIOStorage creates a new MinIO storage client.
func NewMinIOStorage(cfg *config.MinIOConfig, logger *zap.Logger, opts MinIOOptions) (*MinIOStorage, error) {
	if cfg == nil {
		return nil, errors.New("minio config is required")
	}
	if logger == nil {
		return nil, errors.New("logger is required")
	}
	if cfg.Endpoint == "" {
		return nil, errors.New("minio endpoint is required")
	}
	if cfg.Bucket == "" {
		return nil, errors.New("minio bucket is required")
	}

	// Set defaults
	if opts.UploadExpiry == 0 {
		opts.UploadExpiry = 90 * 24 * time.Hour // 90 days default
	}

	// Create MinIO client
	client, err := minio.New(cfg.Endpoint, &minio.Options{
		Creds:  credentials.NewStaticV4(cfg.AccessKey, cfg.SecretKey, ""),
		Secure: cfg.UseSSL,
	})
	if err != nil {
		logger.Error("failed to create minio client",
			zap.String("endpoint", cfg.Endpoint),
			zap.Error(err),
		)
		return nil, fmt.Errorf("create minio client: %w", err)
	}

	logger.Info("minio client created",
		zap.String("endpoint", cfg.Endpoint),
		zap.String("bucket", cfg.Bucket),
		zap.Bool("use_ssl", cfg.UseSSL),
	)

	return &MinIOStorage{
		client: client,
		bucket: cfg.Bucket,
		logger: logger,
		opts:   opts,
	}, nil
}

// Upload uploads an object to cold storage.
// The object is stored with user metadata containing the upload timestamp for TTL tracking.
func (s *MinIOStorage) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	if key == "" {
		return errors.New("key is required")
	}
	if len(data) == 0 {
		return errors.New("data is required")
	}

	// Create a reader from the data
	reader := bytes.NewReader(data)

	// Upload with metadata containing upload time for TTL tracking
	uploadTime := time.Now().UTC().UnixNano()
	metadata := map[string]string{
		"X-Amz-Meta-Upload-Time": fmt.Sprintf("%d", uploadTime),
	}

	_, err := s.client.PutObject(ctx, s.bucket, key, reader, int64(len(data)), minio.PutObjectOptions{
		ContentType: contentType,
		UserMetadata: metadata,
	})
	if err != nil {
		s.logger.Error("failed to upload object",
			zap.String("key", key),
			zap.String("bucket", s.bucket),
			zap.Int("size", len(data)),
			zap.Error(err),
		)
		return fmt.Errorf("upload object %s: %w", key, err)
	}

	s.logger.Debug("object uploaded",
		zap.String("key", key),
		zap.String("bucket", s.bucket),
		zap.Int("size", len(data)),
		zap.String("content_type", contentType),
	)
	return nil
}

// Download downloads an object from cold storage by key.
func (s *MinIOStorage) Download(ctx context.Context, key string) ([]byte, error) {
	if key == "" {
		return nil, errors.New("key is required")
	}

	obj, err := s.client.GetObject(ctx, s.bucket, key, minio.GetObjectOptions{})
	if err != nil {
		s.logger.Error("failed to get object",
			zap.String("key", key),
			zap.String("bucket", s.bucket),
			zap.Error(err),
		)
		return nil, fmt.Errorf("get object %s: %w", key, err)
	}
	defer obj.Close()

	// Read the object content
	data, err := io.ReadAll(obj)
	if err != nil {
		s.logger.Error("failed to read object",
			zap.String("key", key),
			zap.String("bucket", s.bucket),
			zap.Error(err),
		)
		return nil, fmt.Errorf("read object %s: %w", key, err)
	}

	// Check if object exists by trying to stat it
	stat, err := obj.Stat()
	if err != nil {
		// Check if it's a "object not found" error
		if minio.ToErrorResponse(err).Code == "NoSuchKey" {
			return nil, ErrL1ObjectNotFound
		}
		s.logger.Error("failed to stat object",
			zap.String("key", key),
			zap.Error(err),
		)
		return nil, fmt.Errorf("stat object %s: %w", key, err)
	}

	s.logger.Debug("object downloaded",
		zap.String("key", key),
		zap.String("bucket", s.bucket),
		zap.Int64("size", stat.Size),
	)
	return data, nil
}

// GeneratePresignedURL generates a presigned URL for direct object access.
// The URL expires after the specified expiry duration (default: 1 hour).
func (s *MinIOStorage) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if key == "" {
		return "", errors.New("key is required")
	}
	if expiry <= 0 {
		expiry = DefaultPresignExpiry
	}

	// First check if the object exists
	exists, err := s.Exists(ctx, key)
	if err != nil {
		return "", fmt.Errorf("check object existence: %w", err)
	}
	if !exists {
		return "", ErrL1ObjectNotFound
	}

	// Generate presigned URL
	url, err := s.client.PresignedGetObject(ctx, s.bucket, key, expiry, nil)
	if err != nil {
		s.logger.Error("failed to generate presigned URL",
			zap.String("key", key),
			zap.String("bucket", s.bucket),
			zap.Duration("expiry", expiry),
			zap.Error(err),
		)
		return "", fmt.Errorf("generate presigned URL for %s: %w", key, err)
	}

	s.logger.Debug("presigned URL generated",
		zap.String("key", key),
		zap.String("bucket", s.bucket),
		zap.Duration("expiry", expiry),
	)
	return url.String(), nil
}

// Delete deletes an object from cold storage.
func (s *MinIOStorage) Delete(ctx context.Context, key string) error {
	if key == "" {
		return errors.New("key is required")
	}

	err := s.client.RemoveObject(ctx, s.bucket, key, minio.RemoveObjectOptions{})
	if err != nil {
		s.logger.Error("failed to delete object",
			zap.String("key", key),
			zap.String("bucket", s.bucket),
			zap.Error(err),
		)
		return fmt.Errorf("delete object %s: %w", key, err)
	}

	s.logger.Debug("object deleted",
		zap.String("key", key),
		zap.String("bucket", s.bucket),
	)
	return nil
}

// ListExpiredObjects returns keys of objects older than the given TTL.
// It lists all objects in the bucket and checks their upload metadata.
func (s *MinIOStorage) ListExpiredObjects(ctx context.Context, olderThan time.Duration) ([]string, error) {
	if olderThan <= 0 {
		return nil, errors.New("olderThan must be positive")
	}

	cutoff := time.Now().UTC().Add(-olderThan).UnixNano()
	var expiredKeys []string

	// List all objects in the bucket
	doneCh := make(chan struct{})
	defer close(doneCh)

	for object := range s.client.ListObjects(ctx, s.bucket, minio.ListObjectsOptions{
		Recursive: true,
	}) {
		if object.Err != nil {
			s.logger.Warn("error listing object",
				zap.String("key", object.Key),
				zap.Error(object.Err),
			)
			continue
		}

		// Get object metadata to check upload time
		stat, err := s.client.StatObject(ctx, s.bucket, object.Key, minio.StatObjectOptions{})
		if err != nil {
			s.logger.Warn("error stat object",
				zap.String("key", object.Key),
				zap.Error(err),
			)
			continue
		}

		// Check upload time from metadata
		uploadTimeStr := stat.Metadata.Get("X-Amz-Meta-Upload-Time")
		if uploadTimeStr == "" {
			// If no upload time metadata, use object creation time as fallback
			uploadTimeStr = stat.Metadata.Get("Date")
		}

		var uploadTime int64
		if uploadTimeStr != "" {
			uploadTime, _ = strconv.ParseInt(uploadTimeStr, 10, 64)
		}

		// If upload time is before cutoff, mark as expired
		if uploadTime > 0 && uploadTime < cutoff {
			expiredKeys = append(expiredKeys, object.Key)
		}
	}

	s.logger.Info("found expired objects",
		zap.String("bucket", s.bucket),
		zap.Duration("older_than", olderThan),
		zap.Int("count", len(expiredKeys)),
	)
	return expiredKeys, nil
}

// Exists checks if an object exists in cold storage.
func (s *MinIOStorage) Exists(ctx context.Context, key string) (bool, error) {
	if key == "" {
		return false, errors.New("key is required")
	}

	_, err := s.client.StatObject(ctx, s.bucket, key, minio.StatObjectOptions{})
	if err != nil {
		errResp := minio.ToErrorResponse(err)
		if errResp.Code == "NoSuchKey" || errResp.Code == "AccessDenied" {
			return false, nil
		}
		s.logger.Warn("error checking object existence",
			zap.String("key", key),
			zap.Error(err),
		)
		return false, fmt.Errorf("stat object %s: %w", key, err)
	}
	return true, nil
}

// BucketName returns the configured bucket name.
func (s *MinIOStorage) BucketName() string {
	return s.bucket
}

// EnsureBucket ensures the bucket exists, creating it if necessary.
func (s *MinIOStorage) EnsureBucket(ctx context.Context) error {
	exists, err := s.client.BucketExists(ctx, s.bucket)
	if err != nil {
		s.logger.Error("failed to check bucket existence",
			zap.String("bucket", s.bucket),
			zap.Error(err),
		)
		return fmt.Errorf("check bucket %s: %w", s.bucket, err)
	}

	if !exists {
		err = s.client.MakeBucket(ctx, s.bucket, minio.MakeBucketOptions{})
		if err != nil {
			s.logger.Error("failed to create bucket",
				zap.String("bucket", s.bucket),
				zap.Error(err),
			)
			return fmt.Errorf("create bucket %s: %w", s.bucket, err)
		}
		s.logger.Info("bucket created",
			zap.String("bucket", s.bucket),
		)
	}

	return nil
}

// Verify interface implementation at compile time.
var _ = (interface {
	EnsureBucket(ctx context.Context) error
})((*MinIOStorage)(nil))
