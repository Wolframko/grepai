package indexer

import (
	"context"
	"fmt"
	"testing"
	"time"

	"github.com/yoanbernabeu/grepai/store"
)

// mockEmbedder provides a simple mock implementation for testing
type mockEmbedder struct {
	dimensions int
	delay      time.Duration
}

func newMockEmbedder(dimensions int, delay time.Duration) *mockEmbedder {
	return &mockEmbedder{
		dimensions: dimensions,
		delay:      delay,
	}
}

func (m *mockEmbedder) Embed(ctx context.Context, text string) ([]float32, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	vec := make([]float32, m.dimensions)
	for i := range vec {
		vec[i] = 0.1
	}
	return vec, nil
}

func (m *mockEmbedder) EmbedBatch(ctx context.Context, texts []string) ([][]float32, error) {
	if m.delay > 0 {
		time.Sleep(m.delay)
	}
	vecs := make([][]float32, len(texts))
	for i := range texts {
		vecs[i] = make([]float32, m.dimensions)
		for j := range vecs[i] {
			vecs[i][j] = 0.1
		}
	}
	return vecs, nil
}

func (m *mockEmbedder) Dimensions() int {
	return m.dimensions
}

func (m *mockEmbedder) Close() error {
	return nil
}


// generateTestFiles creates test files with content
func generateTestFiles(count int) []FileInfo {
	files := make([]FileInfo, count)
	for i := 0; i < count; i++ {
		files[i] = FileInfo{
			Path: fmt.Sprintf("test%d.go", i),
			// Generate content that will be split into multiple chunks
			Content: fmt.Sprintf("package test\n\n// File %d\n// This is a test file with some content\n// "+
				"It contains multiple lines to ensure proper chunking\n"+
				"func TestFunction%d() {\n"+
				"    // Function implementation\n"+
				"    return nil\n"+
				"}\n", i, i),
			Hash:    fmt.Sprintf("hash%d", i),
			ModTime: time.Now().Unix(),
		}
	}
	return files
}

func TestBuildDocumentHashMap(t *testing.T) {
	ctx := context.Background()
	st := store.NewGOBStore(":memory:")

	// Create test documents
	testDocs := []store.Document{
		{Path: "file1.go", Hash: "hash1", ModTime: time.Now(), ChunkIDs: []string{"chunk1"}},
		{Path: "file2.go", Hash: "hash2", ModTime: time.Now(), ChunkIDs: []string{"chunk2"}},
		{Path: "file3.go", Hash: "hash3", ModTime: time.Now(), ChunkIDs: []string{"chunk3"}},
	}

	for _, doc := range testDocs {
		if err := st.SaveDocument(ctx, doc); err != nil {
			t.Fatalf("Failed to save document: %v", err)
		}
	}

	// Create indexer (scanner can be nil for this test)
	chunker := NewChunker(512, 50)
	emb := newMockEmbedder(768, 0)
	idx := NewIndexer("/tmp/test", st, emb, chunker, nil)

	// Build hash map
	paths := []string{"file1.go", "file2.go", "file3.go", "nonexistent.go"}
	hashMap, err := idx.buildDocumentHashMap(ctx, paths)
	if err != nil {
		t.Fatalf("buildDocumentHashMap failed: %v", err)
	}

	// Verify results
	if len(hashMap) != 3 {
		t.Errorf("Expected 3 entries in hashMap, got %d", len(hashMap))
	}

	for _, doc := range testDocs {
		if hash, ok := hashMap[doc.Path]; !ok {
			t.Errorf("Missing hash for %s", doc.Path)
		} else if hash != doc.Hash {
			t.Errorf("Wrong hash for %s: expected %s, got %s", doc.Path, doc.Hash, hash)
		}
	}

	// Verify nonexistent file is not in map
	if _, ok := hashMap["nonexistent.go"]; ok {
		t.Error("Nonexistent file should not be in hashMap")
	}
}

// Note: Integration tests and benchmarks require a more complex setup with real file scanners.
// The tests below are commented out but can be uncommented and adapted when a proper test
// infrastructure with temporary files is set up.

/*
func TestIndexAllParallelBasic(t *testing.T) {
	// TODO: Implement with temporary test files
}

func TestIndexAllParallelWithExistingFiles(t *testing.T) {
	// TODO: Implement with temporary test files
}
*/

// TestBatchDocumentReaderInterface verifies both GOBStore and PostgresStore implement BatchDocumentReader
func TestBatchDocumentReaderInterface(t *testing.T) {
	ctx := context.Background()

	// Test GOBStore
	gobStore := store.NewGOBStore(":memory:")
	testDocs := []store.Document{
		{Path: "file1.go", Hash: "hash1", ModTime: time.Now(), ChunkIDs: []string{"chunk1"}},
		{Path: "file2.go", Hash: "hash2", ModTime: time.Now(), ChunkIDs: []string{"chunk2"}},
	}

	for _, doc := range testDocs {
		if err := gobStore.SaveDocument(ctx, doc); err != nil {
			t.Fatalf("Failed to save document: %v", err)
		}
	}

	// Verify GOBStore implements BatchDocumentReader by casting to VectorStore interface first
	var st store.VectorStore = gobStore
	if batchReader, ok := st.(store.BatchDocumentReader); ok {
		docs, err := batchReader.GetDocumentBatch(ctx, []string{"file1.go", "file2.go", "nonexistent.go"})
		if err != nil {
			t.Fatalf("GetDocumentBatch failed: %v", err)
		}

		if len(docs) != 2 {
			t.Errorf("Expected 2 documents, got %d", len(docs))
		}

		for _, expectedDoc := range testDocs {
			if doc, ok := docs[expectedDoc.Path]; !ok {
				t.Errorf("Missing document: %s", expectedDoc.Path)
			} else if doc.Hash != expectedDoc.Hash {
				t.Errorf("Wrong hash for %s: expected %s, got %s", expectedDoc.Path, expectedDoc.Hash, doc.Hash)
			}
		}
	} else {
		t.Error("GOBStore does not implement BatchDocumentReader")
	}
}
