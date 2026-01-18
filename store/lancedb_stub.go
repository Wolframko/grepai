//go:build !lancedb
// +build !lancedb

package store

import (
	"context"
	"fmt"
)

// LanceDBStore stub when LanceDB is not compiled in
type LanceDBStore struct{}

func NewLanceDBStore(dbPath string, dimensions int) (*LanceDBStore, error) {
	return nil, fmt.Errorf("LanceDB backend is not available. Rebuild with '-tags lancedb' to enable LanceDB support")
}

func (s *LanceDBStore) Load(ctx context.Context) error {
	return fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) SaveChunks(ctx context.Context, chunks []Chunk) error {
	return fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) DeleteByFile(ctx context.Context, filePath string) error {
	return fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) Search(ctx context.Context, queryVector []float32, limit int) ([]SearchResult, error) {
	return nil, fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) GetDocument(ctx context.Context, filePath string) (*Document, error) {
	return nil, fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) SaveDocument(ctx context.Context, doc Document) error {
	return fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) DeleteDocument(ctx context.Context, filePath string) error {
	return fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) ListDocuments(ctx context.Context) ([]string, error) {
	return nil, fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) Persist(ctx context.Context) error {
	return fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) Close() error {
	return nil
}

func (s *LanceDBStore) GetStats(ctx context.Context) (*IndexStats, error) {
	return nil, fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) ListFilesWithStats(ctx context.Context) ([]FileStats, error) {
	return nil, fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) GetChunksForFile(ctx context.Context, filePath string) ([]Chunk, error) {
	return nil, fmt.Errorf("LanceDB backend is not available")
}

func (s *LanceDBStore) GetAllChunks(ctx context.Context) ([]Chunk, error) {
	return nil, fmt.Errorf("LanceDB backend is not available")
}
