package solver

import (
	"context"
	"slices"
	"testing"
	"time"

	"github.com/moby/buildkit/session"
	"github.com/moby/buildkit/util/compression"
	digest "github.com/opencontainers/go-digest"
)

func TestCompareCacheRecord(t *testing.T) {
	now := time.Now()
	a := &CacheRecord{CreatedAt: now, Priority: 1}
	b := &CacheRecord{CreatedAt: now, Priority: 2}
	c := &CacheRecord{CreatedAt: now.Add(1 * time.Second), Priority: 1}
	d := &CacheRecord{CreatedAt: now.Add(-1 * time.Second), Priority: 1}

	records := []*CacheRecord{b, nil, d, a, c, nil}
	slices.SortFunc(records, compareCacheRecord)

	names := map[*CacheRecord]string{
		a:   "a",
		b:   "b",
		c:   "c",
		d:   "d",
		nil: "nil",
	}
	var got []string
	for _, r := range records {
		got = append(got, names[r])
	}
	want := []string{"c", "a", "b", "d", "nil", "nil"}
	if !slices.Equal(got, want) {
		t.Fatalf("unexpected order: got %v, want %v", got, want)
	}
}

// mockBackend is a mock implementation of CacheKeyStorage for testing
type mockBackend struct {
	loadFunc func(string, string) (CacheResult, error)
}

func (m *mockBackend) Exists(id string) bool {
	return true
}

func (m *mockBackend) Walk(fn func(id string) error) error {
	return nil
}

func (m *mockBackend) WalkResults(id string, fn func(CacheResult) error) error {
	return nil
}

func (m *mockBackend) Load(id string, resultID string) (CacheResult, error) {
	if m.loadFunc != nil {
		return m.loadFunc(id, resultID)
	}
	return CacheResult{}, nil
}

func (m *mockBackend) AddResult(id string, res CacheResult) error {
	return nil
}

func (m *mockBackend) Release(resultID string) error {
	return nil
}

func (m *mockBackend) WalkIDsByResult(resultID string, fn func(string) error) error {
	return nil
}

func (m *mockBackend) AddLink(id string, link CacheInfoLink, target string) error {
	return nil
}

func (m *mockBackend) WalkLinks(id string, link CacheInfoLink, fn func(id string) error) error {
	return nil
}

func (m *mockBackend) HasLink(id string, link CacheInfoLink, target string) bool {
	return false
}

func (m *mockBackend) WalkBacklinks(id string, fn func(id string, link CacheInfoLink) error) error {
	return nil
}

// mockResultStorage is a mock implementation of CacheResultStorage for testing
type mockResultStorage struct{}

func (m *mockResultStorage) Save(Result, time.Time) (CacheResult, error) {
	return CacheResult{}, nil
}

func (m *mockResultStorage) Load(ctx context.Context, res CacheResult) (Result, error) {
	return nil, nil
}

func (m *mockResultStorage) LoadRemotes(ctx context.Context, res CacheResult, compression *compression.Config, s session.Group) ([]*Remote, error) {
	return nil, nil
}

func (m *mockResultStorage) Exists(ctx context.Context, id string) bool {
	return true
}

// mockExporterTarget is a mock implementation of CacheExporterTarget for testing
type mockExporterTarget struct {
	visited map[any]bool
	records []CacheExporterRecord
}

func newMockExporterTarget() *mockExporterTarget {
	return &mockExporterTarget{
		visited: make(map[any]bool),
		records: make([]CacheExporterRecord, 0),
	}
}

func (m *mockExporterTarget) Add(dgst digest.Digest) CacheExporterRecord {
	rec := &mockExporterRecord{digest: dgst}
	m.records = append(m.records, rec)
	return rec
}

func (m *mockExporterTarget) Visit(target any) {
	m.visited[target] = true
}

func (m *mockExporterTarget) Visited(target any) bool {
	return m.visited[target]
}

// mockExporterRecord is a mock implementation of CacheExporterRecord for testing
type mockExporterRecord struct {
	digest digest.Digest
}

func (m *mockExporterRecord) AddResult(vtx digest.Digest, index int, createdAt time.Time, result *Remote) {
	// Mock implementation - do nothing
}

func (m *mockExporterRecord) LinkFrom(src CacheExporterRecord, index int, selector string) {
	// Mock implementation - do nothing
}

func TestExporterExportToWithErrNotFound(t *testing.T) {
	// Create a mock backend that returns ErrNotFound
	mockBackend := &mockBackend{
		loadFunc: func(id string, resultID string) (CacheResult, error) {
			return CacheResult{}, ErrNotFound
		},
	}

	// Create a mock cache manager
	mockCacheManager := &cacheManager{
		id:      "test-cache-manager",
		backend: mockBackend,
		results: &mockResultStorage{},
	}

	// Create a cache key
	cacheKey := &CacheKey{
		ID:     "test-cache-key",
		digest: digest.Digest("sha256:test-digest"),
		vtx:    digest.Digest("sha256:test-vtx"),
		output: Index(0),
		ids:    map[*cacheManager]string{mockCacheManager: "test-cache-key"},
		deps:   [][]CacheKeyWithSelector{}, // No dependencies
	}

	// Create a cache record with non-nil e.record
	cacheRecord := &CacheRecord{
		ID:           "test-record-id",
		Size:         1024,
		CreatedAt:    time.Now(),
		Priority:     1,
		cacheManager: mockCacheManager,
		key:          cacheKey,
	}

	// Create the exporter with a non-nil record
	exporter := &exporter{
		k:      cacheKey,
		record: cacheRecord,
	}

	// Create the export target
	target := newMockExporterTarget()

	// Create export options
	exportOpt := CacheExportOpt{
		ResolveRemotes: func(ctx context.Context, res Result) ([]*Remote, error) {
			return []*Remote{}, nil
		},
		Mode:            CacheExportModeMax,
		Session:         session.NewGroup(),
		CompressionOpt:  nil,
		ExportRoots:     true,
		IgnoreBacklinks: false,
	}

	// Test the ExportTo method
	ctx := context.Background()
	records, err := exporter.ExportTo(ctx, target, exportOpt)

	// Verify that no error occurred and we got some records back
	if err != nil {
		t.Fatalf("ExportTo returned an error: %v", err)
	}

	// The exporter should handle ErrNotFound gracefully and return records
	// even when the backend.Load returns ErrNotFound
	if len(records) == 0 {
		t.Fatalf("Expected at least one record, got %d", len(records))
	}

	// Verify that the target was visited
	if !target.Visited(exporter) {
		t.Fatalf("Expected exporter to be visited")
	}
}
