//go:build lancedb
// +build lancedb

package store

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"path/filepath"
	"sync"
	"time"

	"github.com/apache/arrow/go/v17/arrow"
	"github.com/apache/arrow/go/v17/arrow/array"
	"github.com/apache/arrow/go/v17/arrow/memory"
	"github.com/lancedb/lancedb-go/pkg/contracts"
	"github.com/lancedb/lancedb-go/pkg/lancedb"
)

const (
	chunksTableName    = "chunks"
	documentsTableName = "documents"
)

type LanceDBStore struct {
	conn     contracts.IConnection
	dbPath   string
	mu       sync.RWMutex
	metadata map[string]Document // Cache for document metadata
	dims     int                 // Vector dimensions
}

func NewLanceDBStore(dbPath string, dimensions int) (*LanceDBStore, error) {
	// Ensure the directory exists
	if err := os.MkdirAll(dbPath, 0755); err != nil {
		return nil, fmt.Errorf("failed to create lancedb directory: %w", err)
	}

	return &LanceDBStore{
		dbPath:   dbPath,
		metadata: make(map[string]Document),
		dims:     dimensions,
	}, nil
}

func (s *LanceDBStore) Load(ctx context.Context) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Connect to LanceDB
	conn, err := lancedb.Connect(ctx, s.dbPath, nil)
	if err != nil {
		return fmt.Errorf("failed to connect to lancedb: %w", err)
	}
	s.conn = conn

	// Load document metadata into memory cache
	if err := s.loadDocumentMetadata(ctx); err != nil {
		return fmt.Errorf("failed to load document metadata: %w", err)
	}

	return nil
}

func (s *LanceDBStore) loadDocumentMetadata(ctx context.Context) error {
	// Check if documents table exists
	tables, err := s.conn.TableNames(ctx)
	if err != nil {
		return fmt.Errorf("failed to list tables: %w", err)
	}

	hasDocTable := false
	for _, name := range tables {
		if name == documentsTableName {
			hasDocTable = true
			break
		}
	}

	if !hasDocTable {
		// No documents table yet, start fresh
		return nil
	}

	// Open documents table
	docTable, err := s.conn.OpenTable(ctx, documentsTableName)
	if err != nil {
		return fmt.Errorf("failed to open documents table: %w", err)
	}
	defer docTable.Close()

	// Query all documents
	emptyVec := make([]float32, 1)
	results, err := docTable.VectorSearch(ctx, "vector", emptyVec, 1000000)
	if err != nil {
		return nil // Table might be empty
	}

	// Load into cache
	for _, record := range results {
		path, _ := record["path"].(string)
		hash, _ := record["hash"].(string)
		chunkIDsStr, _ := record["chunk_ids"].(string)

		var chunkIDs []string
		if chunkIDsStr != "" {
			_ = json.Unmarshal([]byte(chunkIDsStr), &chunkIDs)
		}

		// Parse mod_time
		modTime := time.Now()
		if mt, ok := record["mod_time"].(string); ok {
			modTime, _ = time.Parse(time.RFC3339, mt)
		}

		s.metadata[path] = Document{
			Path:     path,
			Hash:     hash,
			ModTime:  modTime,
			ChunkIDs: chunkIDs,
		}
	}

	return nil
}

func (s *LanceDBStore) SaveChunks(ctx context.Context, chunks []Chunk) error {
	if len(chunks) == 0 {
		return nil
	}

	s.mu.Lock()
	defer s.mu.Unlock()

	// Get or create chunks table
	tbl, err := s.getOrCreateChunksTable(ctx)
	if err != nil {
		return fmt.Errorf("failed to get chunks table: %w", err)
	}
	defer tbl.Close()

	// Convert chunks to Arrow record
	record, err := s.chunksToArrowRecord(chunks)
	if err != nil {
		return fmt.Errorf("failed to convert chunks to arrow: %w", err)
	}
	defer record.Release()

	// Add record to table
	if err := tbl.Add(ctx, record, nil); err != nil {
		return fmt.Errorf("failed to add chunks: %w", err)
	}

	return nil
}

func (s *LanceDBStore) DeleteByFile(ctx context.Context, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Get document to find chunk IDs
	doc, ok := s.metadata[filePath]
	if !ok {
		return nil // No document to delete
	}

	// Open chunks table
	tbl, err := s.conn.OpenTable(ctx, chunksTableName)
	if err != nil {
		return fmt.Errorf("failed to open chunks table: %w", err)
	}
	defer tbl.Close()

	// Delete each chunk
	for _, chunkID := range doc.ChunkIDs {
		predicate := fmt.Sprintf("id = '%s'", chunkID)
		if err := tbl.Delete(ctx, predicate); err != nil {
			return fmt.Errorf("failed to delete chunk %s: %w", chunkID, err)
		}
	}

	return nil
}

func (s *LanceDBStore) Search(ctx context.Context, queryVector []float32, limit int) ([]SearchResult, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Open chunks table
	tbl, err := s.conn.OpenTable(ctx, chunksTableName)
	if err != nil {
		return nil, fmt.Errorf("failed to open chunks table: %w", err)
	}
	defer tbl.Close()

	// Perform vector search
	results, err := tbl.VectorSearch(ctx, "vector", queryVector, limit)
	if err != nil {
		return nil, fmt.Errorf("failed to execute search: %w", err)
	}

	// Convert results
	searchResults := make([]SearchResult, 0, len(results))
	for _, record := range results {
		chunk := Chunk{
			ID:        getString(record, "id"),
			FilePath:  getString(record, "file_path"),
			StartLine: getInt(record, "start_line"),
			EndLine:   getInt(record, "end_line"),
			Content:   getString(record, "content"),
			Vector:    getFloatSlice(record, "vector"),
			Hash:      getString(record, "hash"),
			UpdatedAt: getTime(record, "updated_at"),
		}

		// Calculate cosine similarity score
		score := cosineSimilarity(queryVector, chunk.Vector)

		searchResults = append(searchResults, SearchResult{
			Chunk: chunk,
			Score: score,
		})
	}

	return searchResults, nil
}

func (s *LanceDBStore) GetDocument(ctx context.Context, filePath string) (*Document, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.metadata[filePath]
	if !ok {
		return nil, nil
	}

	return &doc, nil
}

func (s *LanceDBStore) SaveDocument(ctx context.Context, doc Document) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Update cache
	s.metadata[doc.Path] = doc

	// Persist to LanceDB
	tbl, err := s.getOrCreateDocumentsTable(ctx)
	if err != nil {
		return fmt.Errorf("failed to get documents table: %w", err)
	}
	defer tbl.Close()

	// Convert to Arrow record
	record, err := s.documentsToArrowRecord([]Document{doc})
	if err != nil {
		return fmt.Errorf("failed to convert document to arrow: %w", err)
	}
	defer record.Release()

	if err := tbl.Add(ctx, record, nil); err != nil {
		return fmt.Errorf("failed to add document: %w", err)
	}

	return nil
}

func (s *LanceDBStore) DeleteDocument(ctx context.Context, filePath string) error {
	s.mu.Lock()
	defer s.mu.Unlock()

	// Remove from cache
	delete(s.metadata, filePath)

	// Delete from LanceDB
	tbl, err := s.conn.OpenTable(ctx, documentsTableName)
	if err != nil {
		return fmt.Errorf("failed to open documents table: %w", err)
	}
	defer tbl.Close()

	predicate := fmt.Sprintf("path = '%s'", filePath)
	if err := tbl.Delete(ctx, predicate); err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	return nil
}

func (s *LanceDBStore) ListDocuments(ctx context.Context) ([]string, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	paths := make([]string, 0, len(s.metadata))
	for path := range s.metadata {
		paths = append(paths, path)
	}

	return paths, nil
}

func (s *LanceDBStore) Persist(ctx context.Context) error {
	// LanceDB automatically persists data
	return nil
}

func (s *LanceDBStore) Close() error {
	s.mu.Lock()
	defer s.mu.Unlock()

	if s.conn != nil {
		return s.conn.Close()
	}
	return nil
}

func (s *LanceDBStore) GetStats(ctx context.Context) (*IndexStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	var lastUpdated time.Time
	totalChunks := int64(0)

	// Try to count chunks
	if tbl, err := s.conn.OpenTable(ctx, chunksTableName); err == nil {
		defer tbl.Close()
		totalChunks, _ = tbl.Count(ctx)

		// Get latest updated_at by querying all chunks
		emptyVec := make([]float32, s.dims)
		if results, err := tbl.VectorSearch(ctx, "vector", emptyVec, 1000000); err == nil {
			for _, record := range results {
				if t := getTime(record, "updated_at"); t.After(lastUpdated) {
					lastUpdated = t
				}
			}
		}
	}

	// Estimate database size
	dbSize := int64(0)
	filepath.Walk(s.dbPath, func(_ string, info os.FileInfo, err error) error {
		if err == nil && !info.IsDir() {
			dbSize += info.Size()
		}
		return nil
	})

	return &IndexStats{
		TotalFiles:  len(s.metadata),
		TotalChunks: int(totalChunks),
		IndexSize:   dbSize,
		LastUpdated: lastUpdated,
	}, nil
}

func (s *LanceDBStore) ListFilesWithStats(ctx context.Context) ([]FileStats, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	stats := make([]FileStats, 0, len(s.metadata))
	for _, doc := range s.metadata {
		stats = append(stats, FileStats{
			Path:       doc.Path,
			ChunkCount: len(doc.ChunkIDs),
			ModTime:    doc.ModTime,
		})
	}

	return stats, nil
}

func (s *LanceDBStore) GetChunksForFile(ctx context.Context, filePath string) ([]Chunk, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	doc, ok := s.metadata[filePath]
	if !ok {
		return nil, nil
	}

	// Open chunks table
	tbl, err := s.conn.OpenTable(ctx, chunksTableName)
	if err != nil {
		return nil, fmt.Errorf("failed to open chunks table: %w", err)
	}
	defer tbl.Close()

	var chunks []Chunk
	for _, chunkID := range doc.ChunkIDs {
		// Query by chunk ID
		predicate := fmt.Sprintf("id = '%s'", chunkID)
		results, err := tbl.SelectWithFilter(ctx, predicate)
		if err != nil {
			continue
		}

		for _, record := range results {
			chunks = append(chunks, Chunk{
				ID:        getString(record, "id"),
				FilePath:  getString(record, "file_path"),
				StartLine: getInt(record, "start_line"),
				EndLine:   getInt(record, "end_line"),
				Content:   getString(record, "content"),
				Vector:    getFloatSlice(record, "vector"),
				Hash:      getString(record, "hash"),
				UpdatedAt: getTime(record, "updated_at"),
			})
		}
	}

	return chunks, nil
}

func (s *LanceDBStore) GetAllChunks(ctx context.Context) ([]Chunk, error) {
	s.mu.RLock()
	defer s.mu.RUnlock()

	// Open chunks table
	tbl, err := s.conn.OpenTable(ctx, chunksTableName)
	if err != nil {
		return nil, fmt.Errorf("failed to open chunks table: %w", err)
	}
	defer tbl.Close()

	// Query all chunks
	results, err := tbl.Select(ctx, contracts.QueryConfig{})
	if err != nil {
		return nil, fmt.Errorf("failed to query all chunks: %w", err)
	}

	chunks := make([]Chunk, 0, len(results))
	for _, record := range results {
		chunks = append(chunks, Chunk{
			ID:        getString(record, "id"),
			FilePath:  getString(record, "file_path"),
			StartLine: getInt(record, "start_line"),
			EndLine:   getInt(record, "end_line"),
			Content:   getString(record, "content"),
			Vector:    getFloatSlice(record, "vector"),
			Hash:      getString(record, "hash"),
			UpdatedAt: getTime(record, "updated_at"),
		})
	}

	return chunks, nil
}

// Helper functions for creating Arrow schemas and records

func (s *LanceDBStore) getOrCreateChunksTable(ctx context.Context) (contracts.ITable, error) {
	// Try to open existing table
	tbl, err := s.conn.OpenTable(ctx, chunksTableName)
	if err == nil {
		return tbl, nil
	}

	// Create new table with Arrow schema
	schema := s.createChunksSchema()
	tbl, err = s.conn.CreateTable(ctx, chunksTableName, schema)
	if err != nil {
		return nil, fmt.Errorf("failed to create chunks table: %w", err)
	}

	return tbl, nil
}

func (s *LanceDBStore) getOrCreateDocumentsTable(ctx context.Context) (contracts.ITable, error) {
	// Try to open existing table
	tbl, err := s.conn.OpenTable(ctx, documentsTableName)
	if err == nil {
		return tbl, nil
	}

	// Create new table with Arrow schema
	schema := s.createDocumentsSchema()
	tbl, err = s.conn.CreateTable(ctx, documentsTableName, schema)
	if err != nil {
		return nil, fmt.Errorf("failed to create documents table: %w", err)
	}

	return tbl, nil
}

func (s *LanceDBStore) createChunksSchema() contracts.ISchema {
	fields := []arrow.Field{
		{Name: "id", Type: arrow.BinaryTypes.String},
		{Name: "file_path", Type: arrow.BinaryTypes.String},
		{Name: "start_line", Type: arrow.PrimitiveTypes.Int64},
		{Name: "end_line", Type: arrow.PrimitiveTypes.Int64},
		{Name: "content", Type: arrow.BinaryTypes.String},
		{Name: "vector", Type: arrow.ListOf(arrow.PrimitiveTypes.Float32)},
		{Name: "hash", Type: arrow.BinaryTypes.String},
		{Name: "updated_at", Type: arrow.BinaryTypes.String},
	}
	return &arrowSchemaWrapper{arrow.NewSchema(fields, nil)}
}

func (s *LanceDBStore) createDocumentsSchema() contracts.ISchema {
	fields := []arrow.Field{
		{Name: "path", Type: arrow.BinaryTypes.String},
		{Name: "hash", Type: arrow.BinaryTypes.String},
		{Name: "mod_time", Type: arrow.BinaryTypes.String},
		{Name: "chunk_ids", Type: arrow.BinaryTypes.String},
		{Name: "vector", Type: arrow.ListOf(arrow.PrimitiveTypes.Float32)},
	}
	return &arrowSchemaWrapper{arrow.NewSchema(fields, nil)}
}

func (s *LanceDBStore) chunksToArrowRecord(chunks []Chunk) (arrow.Record, error) {
	mem := memory.NewGoAllocator()
	schema := s.createChunksSchema().(*arrowSchemaWrapper).schema

	builders := []array.Builder{
		array.NewStringBuilder(mem),                          // id
		array.NewStringBuilder(mem),                          // file_path
		array.NewInt64Builder(mem),                           // start_line
		array.NewInt64Builder(mem),                           // end_line
		array.NewStringBuilder(mem),                          // content
		array.NewListBuilder(mem, arrow.PrimitiveTypes.Float32), // vector
		array.NewStringBuilder(mem),                          // hash
		array.NewStringBuilder(mem),                          // updated_at
	}
	defer func() {
		for _, b := range builders {
			b.Release()
		}
	}()

	for _, chunk := range chunks {
		builders[0].(*array.StringBuilder).Append(chunk.ID)
		builders[1].(*array.StringBuilder).Append(chunk.FilePath)
		builders[2].(*array.Int64Builder).Append(int64(chunk.StartLine))
		builders[3].(*array.Int64Builder).Append(int64(chunk.EndLine))
		builders[4].(*array.StringBuilder).Append(chunk.Content)

		// Vector list
		listBuilder := builders[5].(*array.ListBuilder)
		listBuilder.Append(true)
		valueBuilder := listBuilder.ValueBuilder().(*array.Float32Builder)
		for _, v := range chunk.Vector {
			valueBuilder.Append(v)
		}

		builders[6].(*array.StringBuilder).Append(chunk.Hash)
		builders[7].(*array.StringBuilder).Append(chunk.UpdatedAt.Format(time.RFC3339))
	}

	columns := make([]arrow.Array, len(builders))
	for i, b := range builders {
		columns[i] = b.NewArray()
		defer columns[i].Release()
	}

	return array.NewRecord(schema, columns, int64(len(chunks))), nil
}

func (s *LanceDBStore) documentsToArrowRecord(docs []Document) (arrow.Record, error) {
	mem := memory.NewGoAllocator()
	schema := s.createDocumentsSchema().(*arrowSchemaWrapper).schema

	builders := []array.Builder{
		array.NewStringBuilder(mem),                          // path
		array.NewStringBuilder(mem),                          // hash
		array.NewStringBuilder(mem),                          // mod_time
		array.NewStringBuilder(mem),                          // chunk_ids
		array.NewListBuilder(mem, arrow.PrimitiveTypes.Float32), // vector
	}
	defer func() {
		for _, b := range builders {
			b.Release()
		}
	}()

	for _, doc := range docs {
		chunkIDsJSON, _ := json.Marshal(doc.ChunkIDs)

		builders[0].(*array.StringBuilder).Append(doc.Path)
		builders[1].(*array.StringBuilder).Append(doc.Hash)
		builders[2].(*array.StringBuilder).Append(doc.ModTime.Format(time.RFC3339))
		builders[3].(*array.StringBuilder).Append(string(chunkIDsJSON))

		// Dummy vector
		listBuilder := builders[4].(*array.ListBuilder)
		listBuilder.Append(true)
		valueBuilder := listBuilder.ValueBuilder().(*array.Float32Builder)
		valueBuilder.Append(0.0)
	}

	columns := make([]arrow.Array, len(builders))
	for i, b := range builders {
		columns[i] = b.NewArray()
		defer columns[i].Release()
	}

	return array.NewRecord(schema, columns, int64(len(docs))), nil
}

// Wrapper for Arrow schema to implement contracts.ISchema
type arrowSchemaWrapper struct {
	schema *arrow.Schema
}

func (w *arrowSchemaWrapper) Fields() []arrow.Field {
	return w.schema.Fields()
}

func (w *arrowSchemaWrapper) Field(i int) (arrow.Field, error) {
	if i < 0 || i >= w.schema.NumFields() {
		return arrow.Field{}, fmt.Errorf("field index %d out of range", i)
	}
	return w.schema.Field(i), nil
}

func (w *arrowSchemaWrapper) FieldByName(name string) (arrow.Field, error) {
	indices := w.schema.FieldIndices(name)
	if len(indices) == 0 {
		return arrow.Field{}, fmt.Errorf("field %s not found", name)
	}
	return w.schema.Field(indices[0]), nil
}

func (w *arrowSchemaWrapper) HasField(name string) bool {
	return len(w.schema.FieldIndices(name)) > 0
}

func (w *arrowSchemaWrapper) NumFields() int {
	return w.schema.NumFields()
}

func (w *arrowSchemaWrapper) String() string {
	return w.schema.String()
}

func (w *arrowSchemaWrapper) ToArrowSchema() *arrow.Schema {
	return w.schema
}

// Helper functions for extracting values from records

func getString(record map[string]interface{}, key string) string {
	if v, ok := record[key].(string); ok {
		return v
	}
	return ""
}

func getInt(record map[string]interface{}, key string) int {
	if v, ok := record[key].(int); ok {
		return v
	}
	if v, ok := record[key].(int64); ok {
		return int(v)
	}
	if v, ok := record[key].(float64); ok {
		return int(v)
	}
	return 0
}

func getFloatSlice(record map[string]interface{}, key string) []float32 {
	if v, ok := record[key].([]float32); ok {
		return v
	}
	if v, ok := record[key].([]interface{}); ok {
		result := make([]float32, len(v))
		for i, val := range v {
			if f, ok := val.(float64); ok {
				result[i] = float32(f)
			} else if f, ok := val.(float32); ok {
				result[i] = f
			}
		}
		return result
	}
	return nil
}

func getTime(record map[string]interface{}, key string) time.Time {
	if v, ok := record[key].(string); ok {
		if t, err := time.Parse(time.RFC3339, v); err == nil {
			return t
		}
	}
	return time.Time{}
}
