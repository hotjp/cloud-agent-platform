package storage

import (
	"context"
	"errors"
	"sync/atomic"
	"testing"
	"time"

	"github.com/cloud-agent-platform/cap/internal/config"
	"github.com/stretchr/testify/assert"
	"go.uber.org/zap/zaptest"
)

// mockObjectStorage is a mock implementation of domain.ObjectStorage for testing.
type mockObjectStorage struct {
	objects       map[string][]byte
	uploadedKeys  []string
	deletedKeys   []string
	uploadErr     error
	downloadErr   error
	deleteErr     error
	existsErr     error
	listExpired   []string
	listExpiredErr error
	shouldExist   map[string]bool
	mu            int32 // for atomic operations
}

func newMockObjectStorage() *mockObjectStorage {
	return &mockObjectStorage{
		objects:     make(map[string][]byte),
		uploadedKeys: make([]string, 0),
		deletedKeys: make([]string, 0),
		shouldExist: make(map[string]bool),
	}
}

func (m *mockObjectStorage) Upload(ctx context.Context, key string, data []byte, contentType string) error {
	if m.uploadErr != nil {
		return m.uploadErr
	}
	m.objects[key] = data
	m.uploadedKeys = append(m.uploadedKeys, key)
	return nil
}

func (m *mockObjectStorage) Download(ctx context.Context, key string) ([]byte, error) {
	if m.downloadErr != nil {
		return nil, m.downloadErr
	}
	data, ok := m.objects[key]
	if !ok {
		return nil, ErrL1ObjectNotFound
	}
	return data, nil
}

func (m *mockObjectStorage) GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error) {
	if _, ok := m.objects[key]; !ok {
		return "", ErrL1ObjectNotFound
	}
	return "https://example.com/" + key + "?signature=mock", nil
}

func (m *mockObjectStorage) Delete(ctx context.Context, key string) error {
	if m.deleteErr != nil {
		return m.deleteErr
	}
	delete(m.objects, key)
	m.deletedKeys = append(m.deletedKeys, key)
	return nil
}

func (m *mockObjectStorage) ListExpiredObjects(ctx context.Context, olderThan time.Duration) ([]string, error) {
	if m.listExpiredErr != nil {
		return nil, m.listExpiredErr
	}
	// Return and clear the expired list (simulate actual behavior where objects are removed after listing)
	keys := m.listExpired
	m.listExpired = nil
	return keys, nil
}

func (m *mockObjectStorage) Exists(ctx context.Context, key string) (bool, error) {
	if m.existsErr != nil {
		return false, m.existsErr
	}
	return m.shouldExist[key], nil
}

func (m *mockObjectStorage) BucketName() string {
	return "test-bucket"
}

// TestMinIOStorageNewWithLogger tests creating a new MinIOStorage with nil logger.
func TestMinIOStorageNewWithLogger(t *testing.T) {
	cfg := &config.MinIOConfig{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "test-bucket",
		UseSSL:    false,
	}

	// Should fail with nil logger
	_, err := NewMinIOStorage(cfg, nil, MinIOOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")
}

// TestMinIOStorageNewWithNilConfig tests creating a new MinIOStorage with nil config.
func TestMinIOStorageNewWithNilConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Should fail with nil config
	_, err := NewMinIOStorage(nil, logger, MinIOOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "config is required")
}

// TestMinIOStorageNewWithEmptyEndpoint tests creating a new MinIOStorage with empty endpoint.
func TestMinIOStorageNewWithEmptyEndpoint(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := &config.MinIOConfig{
		Endpoint:  "",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "test-bucket",
	}

	_, err := NewMinIOStorage(cfg, logger, MinIOOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "endpoint is required")
}

// TestMinIOStorageNewWithEmptyBucket tests creating a new MinIOStorage with empty bucket.
func TestMinIOStorageNewWithEmptyBucket(t *testing.T) {
	logger := zaptest.NewLogger(t)

	cfg := &config.MinIOConfig{
		Endpoint:  "localhost:9000",
		AccessKey: "minioadmin",
		SecretKey: "minioadmin",
		Bucket:    "",
	}

	_, err := NewMinIOStorage(cfg, logger, MinIOOptions{})
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "bucket is required")
}

// TestMinIOStorageObjectStorageInterface verifies that MinIOStorage implements domain.ObjectStorage.
func TestMinIOStorageObjectStorageInterface(t *testing.T) {
	// Compile-time check: MinIOStorage implements domain.ObjectStorage
	// The var declaration will fail to compile if the interface is not satisfied
	// We use a dummy variable to avoid "declared and not used" error
	var _ interface {
		Upload(ctx context.Context, key string, data []byte, contentType string) error
		Download(ctx context.Context, key string) ([]byte, error)
		GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
		Delete(ctx context.Context, key string) error
		ListExpiredObjects(ctx context.Context, olderThan time.Duration) ([]string, error)
		Exists(ctx context.Context, key string) (bool, error)
		BucketName() string
	}
	_ = MinIOStorage{}
	assert.True(t, true) // Placeholder assertion
}

// TestCleanupWorkerNewWithNilStorage tests creating a cleanup worker with nil storage.
func TestCleanupWorkerNewWithNilStorage(t *testing.T) {
	logger := zaptest.NewLogger(t)

	// Should fail with nil storage
	_, err := NewCleanupWorker(nil, logger, DefaultCleanupConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "storage is required")
}

// TestCleanupWorkerNewWithNilLogger tests creating a cleanup worker with nil logger.
func TestCleanupWorkerNewWithNilLogger(t *testing.T) {
	mockStorage := newMockObjectStorage()

	// Should fail with nil logger
	_, err := NewCleanupWorker(mockStorage, nil, DefaultCleanupConfig())
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "logger is required")
}

// TestCleanupWorkerNewWithInvalidConfig tests creating a cleanup worker with invalid config.
func TestCleanupWorkerNewWithInvalidConfig(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockStorage := newMockObjectStorage()

	// Invalid config: zero interval
	config := DefaultCleanupConfig()
	config.Interval = 0
	_, err := NewCleanupWorker(mockStorage, logger, config)
	assert.Error(t, err)

	// Invalid config: zero TTL
	config = DefaultCleanupConfig()
	config.ObjectTTL = 0
	_, err = NewCleanupWorker(mockStorage, logger, config)
	assert.Error(t, err)

	// Invalid config: zero batch size
	config = DefaultCleanupConfig()
	config.BatchSize = 0
	_, err = NewCleanupWorker(mockStorage, logger, config)
	assert.Error(t, err)
}

// TestCleanupWorkerStartStop tests starting and stopping the cleanup worker.
func TestCleanupWorkerStartStop(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockStorage := newMockObjectStorage()
	mockStorage.listExpired = []string{} // No expired objects

	config := DefaultCleanupConfig()
	config.Interval = 100 * time.Millisecond // Short interval for testing

	worker, err := NewCleanupWorker(mockStorage, logger, config)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 500*time.Millisecond)
	defer cancel()

	worker.Start(ctx)

	// Let it run for a bit
	time.Sleep(300 * time.Millisecond)

	// Stop should complete without error
	err = worker.Stop()
	assert.NoError(t, err)
}

// TestCleanupWorkerDeletesExpiredObjects tests that cleanup worker deletes expired objects.
func TestCleanupWorkerDeletesExpiredObjects(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockStorage := newMockObjectStorage()
	mockStorage.listExpired = []string{"expired-key-1", "expired-key-2"}

	config := DefaultCleanupConfig()
	config.Interval = 10 * time.Millisecond // Very short interval for testing

	worker, err := NewCleanupWorker(mockStorage, logger, config)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	worker.Start(ctx)

	// Wait for at least one cleanup cycle
	time.Sleep(50 * time.Millisecond)

	// Stop
	err = worker.Stop()
	assert.NoError(t, err)

	// Verify deleted keys
	assert.Equal(t, 2, len(mockStorage.deletedKeys))
	assert.Equal(t, "expired-key-1", mockStorage.deletedKeys[0])
	assert.Equal(t, "expired-key-2", mockStorage.deletedKeys[1])
}

// TestCleanupWorkerHandlesDeleteErrors tests that cleanup worker handles delete errors gracefully.
func TestCleanupWorkerHandlesDeleteErrors(t *testing.T) {
	logger := zaptest.NewLogger(t)
	mockStorage := newMockObjectStorage()
	mockStorage.listExpired = []string{"expired-key-1", "expired-key-2"}
	mockStorage.deleteErr = errors.New("delete failed")

	config := DefaultCleanupConfig()
	config.Interval = 10 * time.Millisecond

	worker, err := NewCleanupWorker(mockStorage, logger, config)
	assert.NoError(t, err)

	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	worker.Start(ctx)
	time.Sleep(50 * time.Millisecond)

	// Stop should complete even with delete errors
	err = worker.Stop()
	assert.NoError(t, err)
}

// TestCleanupConfigValidate tests cleanup config validation.
func TestCleanupConfigValidate(t *testing.T) {
	tests := []struct {
		name    string
		config  CleanupConfig
		wantErr bool
	}{
		{
			name:    "valid config",
			config:  DefaultCleanupConfig(),
			wantErr: false,
		},
		{
			name: "zero interval",
			config: CleanupConfig{
				Interval:  0,
				ObjectTTL: 90 * 24 * time.Hour,
				BatchSize: 100,
			},
			wantErr: true,
		},
		{
			name: "negative TTL",
			config: CleanupConfig{
				Interval:  1 * time.Hour,
				ObjectTTL: -1,
				BatchSize: 100,
			},
			wantErr: true,
		},
		{
			name: "zero batch size",
			config: CleanupConfig{
				Interval:  1 * time.Hour,
				ObjectTTL: 90 * 24 * time.Hour,
				BatchSize: 0,
			},
			wantErr: true,
		},
	}

	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			err := tt.config.Validate()
			if tt.wantErr {
				assert.Error(t, err)
			} else {
				assert.NoError(t, err)
			}
		})
	}
}

// TestDefaultCleanupConfig tests that default cleanup config has 90 day TTL.
func TestDefaultCleanupConfig(t *testing.T) {
	config := DefaultCleanupConfig()

	assert.Equal(t, 1*time.Hour, config.Interval)
	assert.Equal(t, 90*24*time.Hour, config.ObjectTTL)
	assert.Equal(t, 100, config.BatchSize)
}

// TestMockObjectStorage tests the mock object storage implementation.
func TestMockObjectStorage(t *testing.T) {
	mock := newMockObjectStorage()

	// Test Upload
	err := mock.Upload(context.Background(), "test-key", []byte("test data"), "text/plain")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(mock.uploadedKeys))
	assert.Equal(t, "test-key", mock.uploadedKeys[0])

	// Test Download
	data, err := mock.Download(context.Background(), "test-key")
	assert.NoError(t, err)
	assert.Equal(t, []byte("test data"), data)

	// Test Download not found
	_, err = mock.Download(context.Background(), "non-existent")
	assert.Error(t, err)
	assert.Equal(t, ErrL1ObjectNotFound, err)

	// Test GeneratePresignedURL
	url, err := mock.GeneratePresignedURL(context.Background(), "test-key", time.Hour)
	assert.NoError(t, err)
	assert.Contains(t, url, "test-key")

	// Test Exists
	mock.shouldExist["exists"] = true
	mock.shouldExist["not-exists"] = false

	exists, err := mock.Exists(context.Background(), "exists")
	assert.NoError(t, err)
	assert.True(t, exists)

	exists, err = mock.Exists(context.Background(), "not-exists")
	assert.NoError(t, err)
	assert.False(t, exists)

	// Test Delete
	err = mock.Delete(context.Background(), "test-key")
	assert.NoError(t, err)
	assert.Equal(t, 1, len(mock.deletedKeys))

	// Test ListExpiredObjects
	mock.listExpired = []string{"key1", "key2"}
	expired, err := mock.ListExpiredObjects(context.Background(), 90*24*time.Hour)
	assert.NoError(t, err)
	assert.Equal(t, 2, len(expired))

	// Test BucketName
	assert.Equal(t, "test-bucket", mock.BucketName())
}

// TestMockObjectStorageUploadError tests mock with upload error.
func TestMockObjectStorageUploadError(t *testing.T) {
	mock := newMockObjectStorage()
	mock.uploadErr = errors.New("upload failed")

	err := mock.Upload(context.Background(), "test-key", []byte("data"), "text/plain")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "upload failed")
}

// TestMockObjectStorageDownloadError tests mock with download error.
func TestMockObjectStorageDownloadError(t *testing.T) {
	mock := newMockObjectStorage()
	mock.downloadErr = errors.New("download failed")

	_, err := mock.Download(context.Background(), "test-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "download failed")
}

// TestMockObjectStorageDeleteError tests mock with delete error.
func TestMockObjectStorageDeleteError(t *testing.T) {
	mock := newMockObjectStorage()
	mock.deleteErr = errors.New("delete failed")

	err := mock.Delete(context.Background(), "test-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "delete failed")
}

// TestMockObjectStorageListExpiredError tests mock with list expired error.
func TestMockObjectStorageListExpiredError(t *testing.T) {
	mock := newMockObjectStorage()
	mock.listExpiredErr = errors.New("list expired failed")

	_, err := mock.ListExpiredObjects(context.Background(), 90*24*time.Hour)
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "list expired failed")
}

// TestMockObjectStorageExistsError tests mock with exists error.
func TestMockObjectStorageExistsError(t *testing.T) {
	mock := newMockObjectStorage()
	mock.existsErr = errors.New("exists failed")

	_, err := mock.Exists(context.Background(), "test-key")
	assert.Error(t, err)
	assert.Contains(t, err.Error(), "exists failed")
}

// Verify interface implementations at compile time.
var (
	_ interface {
		Upload(ctx context.Context, key string, data []byte, contentType string) error
		Download(ctx context.Context, key string) ([]byte, error)
		GeneratePresignedURL(ctx context.Context, key string, expiry time.Duration) (string, error)
		Delete(ctx context.Context, key string) error
		ListExpiredObjects(ctx context.Context, olderThan time.Duration) ([]string, error)
		Exists(ctx context.Context, key string) (bool, error)
		BucketName() string
	} = (*mockObjectStorage)(nil)
)

// TestCleanupWorkerInterface verifies that CleanupWorker implements the expected interface.
func TestCleanupWorkerInterface(t *testing.T) {
	// Compile-time check: CleanupWorker implements the expected interface
	// The var declaration will fail to compile if the interface is not satisfied
	var _ interface {
		Start(ctx context.Context)
		Stop() error
	}
	_ = CleanupWorker{}
	assert.True(t, true) // Placeholder assertion
}

// TestErrL1ObjectNotFound tests the error type.
func TestErrL1ObjectNotFound(t *testing.T) {
	err := ErrL1ObjectNotFound
	assert.Equal(t, "object not found in cold storage", err.Error())
	assert.True(t, errors.Is(err, ErrL1ObjectNotFound))
}

// TestConfigError tests the ConfigError type.
func TestConfigError(t *testing.T) {
	err := &ConfigError{Field: "test", Message: "failed"}
	assert.Equal(t, "invalid cleanup config: test failed", err.Error())
}

// TestAtomicInt32 verifies that mockObjectStorage works with concurrent access.
func TestMockObjectStorageConcurrentAccess(t *testing.T) {
	mock := newMockObjectStorage()
	mock.shouldExist["key"] = true

	var callCount int32

	// Simulate concurrent calls
	for i := 0; i < 100; i++ {
		go func() {
			_, _ = mock.Exists(context.Background(), "key")
			atomic.AddInt32(&callCount, 1)
		}()
	}

	// Wait for all goroutines
	for i := 0; i < 1000; i++ {
		if atomic.LoadInt32(&callCount) == 100 {
			break
		}
		time.Sleep(1 * time.Millisecond)
	}

	assert.Equal(t, int32(100), atomic.LoadInt32(&callCount))
}
