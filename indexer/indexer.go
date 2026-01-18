package indexer

import (
	"context"
	"fmt"
	"log"
	"time"

	"github.com/yoanbernabeu/grepai/embedder"
	"github.com/yoanbernabeu/grepai/store"
)

type Indexer struct {
	root     string
	store    store.VectorStore
	embedder embedder.Embedder
	chunker  *Chunker
	scanner  *Scanner
}

type IndexStats struct {
	FilesIndexed  int
	FilesSkipped  int
	ChunksCreated int
	FilesRemoved  int
	Duration      time.Duration
}

// ProgressInfo contains progress information for indexing
type ProgressInfo struct {
	Current     int    // Current file number (1-indexed)
	Total       int    // Total number of files
	CurrentFile string // Path of current file being processed
}

// ProgressCallback is called for each file during indexing
type ProgressCallback func(info ProgressInfo)

// fileJob represents a file to be processed by a worker
type fileJob struct {
	file  FileInfo
	index int // for tracking progress
}

// fileResult represents the chunking result for a file
type fileResult struct {
	index      int
	path       string
	chunks     []ChunkInfo // chunks without embeddings
	err        error
	needsIndex bool // whether file needs indexing (hash changed)
}

// chunkBatch represents a batch of chunks ready for embedding
type chunkBatch struct {
	chunks  []ChunkInfo
	fileMap []chunkFileMapping // maps chunk index to file path and chunk index within file
}

// chunkFileMapping tracks which file and chunk index a batch chunk belongs to
type chunkFileMapping struct {
	filePath string
	chunkIdx int
}

func NewIndexer(
	root string,
	st store.VectorStore,
	emb embedder.Embedder,
	chunker *Chunker,
	scanner *Scanner,
) *Indexer {
	return &Indexer{
		root:     root,
		store:    st,
		embedder: emb,
		chunker:  chunker,
		scanner:  scanner,
	}
}

// IndexAll performs a full index of the project (no progress reporting)
func (idx *Indexer) IndexAll(ctx context.Context) (*IndexStats, error) {
	return idx.IndexAllWithProgress(ctx, nil)
}

// IndexAllWithProgress performs a full index with progress reporting
func (idx *Indexer) IndexAllWithProgress(ctx context.Context, onProgress ProgressCallback) (*IndexStats, error) {
	start := time.Now()
	stats := &IndexStats{}

	// Scan all files
	files, skipped, err := idx.scanner.Scan()
	if err != nil {
		return nil, fmt.Errorf("failed to scan files: %w", err)
	}
	stats.FilesSkipped = len(skipped)

	// Get existing documents
	existingDocs, err := idx.store.ListDocuments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	existingMap := make(map[string]bool)
	for _, doc := range existingDocs {
		existingMap[doc] = true
	}

	total := len(files)

	// Index new/modified files
	for i, file := range files {
		// Report progress
		if onProgress != nil {
			onProgress(ProgressInfo{
				Current:     i + 1,
				Total:       total,
				CurrentFile: file.Path,
			})
		}

		// Check if file needs reindexing
		doc, err := idx.store.GetDocument(ctx, file.Path)
		if err != nil {
			return nil, fmt.Errorf("failed to get document %s: %w", file.Path, err)
		}

		if doc != nil && doc.Hash == file.Hash {
			delete(existingMap, file.Path)
			continue // File unchanged
		}

		// Index the file
		chunks, err := idx.IndexFile(ctx, file)
		if err != nil {
			log.Printf("Failed to index %s: %v", file.Path, err)
			continue
		}

		stats.FilesIndexed++
		stats.ChunksCreated += chunks

		delete(existingMap, file.Path)
	}

	// Remove deleted files
	for path := range existingMap {
		if err := idx.RemoveFile(ctx, path); err != nil {
			log.Printf("Failed to remove %s: %v", path, err)
			continue
		}
		stats.FilesRemoved++
	}

	stats.Duration = time.Since(start)
	return stats, nil
}

// IndexAllParallel performs parallel indexing with cross-file embedding batching
func (idx *Indexer) IndexAllParallel(
	ctx context.Context,
	numWorkers int,
	batchSize int,
	onProgress ProgressCallback,
) (*IndexStats, error) {
	start := time.Now()
	stats := &IndexStats{}

	// Scan all files
	files, skipped, err := idx.scanner.Scan()
	if err != nil {
		return nil, fmt.Errorf("failed to scan files: %w", err)
	}
	stats.FilesSkipped = len(skipped)

	if len(files) == 0 {
		stats.Duration = time.Since(start)
		return stats, nil
	}

	// Build file path list for batch document lookup
	filePaths := make([]string, len(files))
	fileInfoMap := make(map[string]FileInfo, len(files))
	for i, file := range files {
		filePaths[i] = file.Path
		fileInfoMap[file.Path] = file
	}

	// Build document hash map using batch operations
	docHashMap, err := idx.buildDocumentHashMap(ctx, filePaths)
	if err != nil {
		return nil, fmt.Errorf("failed to build document hash map: %w", err)
	}

	// Get existing documents for cleanup
	existingDocs, err := idx.store.ListDocuments(ctx)
	if err != nil {
		return nil, fmt.Errorf("failed to list documents: %w", err)
	}

	existingMap := make(map[string]bool)
	for _, doc := range existingDocs {
		existingMap[doc] = true
	}

	// Create channels
	jobs := make(chan fileJob, numWorkers*2)
	results := make(chan fileResult, numWorkers*2)

	// Start workers
	for i := 0; i < numWorkers; i++ {
		go idx.fileWorker(ctx, jobs, results, docHashMap)
	}

	// Start result collector in a goroutine
	collectorDone := make(chan error, 1)
	go func() {
		collectorDone <- idx.resultCollector(
			ctx,
			results,
			len(files),
			batchSize,
			onProgress,
			stats,
			fileInfoMap,
		)
	}()

	// Send jobs to workers
	for i, file := range files {
		select {
		case <-ctx.Done():
			close(jobs)
			return nil, ctx.Err()
		case jobs <- fileJob{file: file, index: i}:
			existingMap[file.Path] = false // mark as still exists
		}
	}
	close(jobs)

	// Wait for result collector to finish
	if err := <-collectorDone; err != nil {
		return nil, fmt.Errorf("collector error: %w", err)
	}
	close(results)

	// Remove deleted files
	for path, shouldRemove := range existingMap {
		if shouldRemove {
			if err := idx.RemoveFile(ctx, path); err != nil {
				log.Printf("Failed to remove %s: %v", path, err)
				continue
			}
			stats.FilesRemoved++
		}
	}

	stats.Duration = time.Since(start)
	return stats, nil
}

// IndexFile indexes a single file
func (idx *Indexer) IndexFile(ctx context.Context, file FileInfo) (int, error) {
	// Remove existing chunks for this file
	if err := idx.store.DeleteByFile(ctx, file.Path); err != nil {
		return 0, fmt.Errorf("failed to delete existing chunks: %w", err)
	}

	// Chunk the file
	chunkInfos := idx.chunker.ChunkWithContext(file.Path, file.Content)
	if len(chunkInfos) == 0 {
		return 0, nil
	}

	// Generate embeddings
	contents := make([]string, len(chunkInfos))
	for i, c := range chunkInfos {
		contents[i] = c.Content
	}

	vectors, err := idx.embedder.EmbedBatch(ctx, contents)
	if err != nil {
		return 0, fmt.Errorf("failed to embed chunks: %w", err)
	}

	// Create store chunks
	now := time.Now()
	chunks := make([]store.Chunk, len(chunkInfos))
	chunkIDs := make([]string, len(chunkInfos))

	for i, info := range chunkInfos {
		chunks[i] = store.Chunk{
			ID:        info.ID,
			FilePath:  info.FilePath,
			StartLine: info.StartLine,
			EndLine:   info.EndLine,
			Content:   info.Content,
			Vector:    vectors[i],
			Hash:      info.Hash,
			UpdatedAt: now,
		}
		chunkIDs[i] = info.ID
	}

	// Save chunks
	if err := idx.store.SaveChunks(ctx, chunks); err != nil {
		return 0, fmt.Errorf("failed to save chunks: %w", err)
	}

	// Save document metadata
	doc := store.Document{
		Path:     file.Path,
		Hash:     file.Hash,
		ModTime:  time.Unix(file.ModTime, 0),
		ChunkIDs: chunkIDs,
	}

	if err := idx.store.SaveDocument(ctx, doc); err != nil {
		return 0, fmt.Errorf("failed to save document: %w", err)
	}

	return len(chunks), nil
}

// RemoveFile removes a file from the index
func (idx *Indexer) RemoveFile(ctx context.Context, path string) error {
	if err := idx.store.DeleteByFile(ctx, path); err != nil {
		return fmt.Errorf("failed to delete chunks: %w", err)
	}

	if err := idx.store.DeleteDocument(ctx, path); err != nil {
		return fmt.Errorf("failed to delete document: %w", err)
	}

	return nil
}

// NeedsReindex checks if a file needs reindexing
func (idx *Indexer) NeedsReindex(ctx context.Context, path string, hash string) (bool, error) {
	doc, err := idx.store.GetDocument(ctx, path)
	if err != nil {
		return false, err
	}

	if doc == nil {
		return true, nil
	}

	return doc.Hash != hash, nil
}

// fileWorker processes files from the jobs channel and sends results to the results channel
func (idx *Indexer) fileWorker(
	ctx context.Context,
	jobs <-chan fileJob,
	results chan<- fileResult,
	docHashMap map[string]string,
) {
	for job := range jobs {
		result := fileResult{
			index:      job.index,
			path:       job.file.Path,
			needsIndex: false,
		}

		// Check if file needs reindexing using the hash map
		existingHash, exists := docHashMap[job.file.Path]
		if !exists || existingHash != job.file.Hash {
			result.needsIndex = true

			// Chunk the file (CPU-bound operation)
			chunks := idx.chunker.ChunkWithContext(job.file.Path, job.file.Content)
			result.chunks = chunks
		}

		results <- result
	}
}

// processBatch generates embeddings for a batch of chunks and saves them to the store
func (idx *Indexer) processBatch(ctx context.Context, batch chunkBatch, fileHashes map[string]string) error {
	if len(batch.chunks) == 0 {
		return nil
	}

	// Extract content for embedding
	contents := make([]string, len(batch.chunks))
	for i, chunk := range batch.chunks {
		contents[i] = chunk.Content
	}

	// Generate embeddings
	vectors, err := idx.embedder.EmbedBatch(ctx, contents)
	if err != nil {
		return fmt.Errorf("failed to embed batch: %w", err)
	}

	// Group chunks by file
	fileChunks := make(map[string][]store.Chunk)
	fileChunkIDs := make(map[string][]string)

	now := time.Now()
	for i, chunkInfo := range batch.chunks {
		mapping := batch.fileMap[i]
		chunk := store.Chunk{
			ID:        chunkInfo.ID,
			FilePath:  chunkInfo.FilePath,
			StartLine: chunkInfo.StartLine,
			EndLine:   chunkInfo.EndLine,
			Content:   chunkInfo.Content,
			Vector:    vectors[i],
			Hash:      chunkInfo.Hash,
			UpdatedAt: now,
		}

		fileChunks[mapping.filePath] = append(fileChunks[mapping.filePath], chunk)
		fileChunkIDs[mapping.filePath] = append(fileChunkIDs[mapping.filePath], chunk.ID)
	}

	// Save chunks and documents for each file
	for filePath, chunks := range fileChunks {
		// Delete old chunks
		if err := idx.store.DeleteByFile(ctx, filePath); err != nil {
			return fmt.Errorf("failed to delete old chunks for %s: %w", filePath, err)
		}

		// Save new chunks
		if err := idx.store.SaveChunks(ctx, chunks); err != nil {
			return fmt.Errorf("failed to save chunks for %s: %w", filePath, err)
		}

		// Save document metadata
		doc := store.Document{
			Path:     filePath,
			Hash:     fileHashes[filePath],
			ModTime:  time.Now(),
			ChunkIDs: fileChunkIDs[filePath],
		}

		if err := idx.store.SaveDocument(ctx, doc); err != nil {
			return fmt.Errorf("failed to save document %s: %w", filePath, err)
		}
	}

	return nil
}

// buildDocumentHashMap builds a map of file paths to their hashes using batch operations if available
func (idx *Indexer) buildDocumentHashMap(ctx context.Context, paths []string) (map[string]string, error) {
	hashMap := make(map[string]string, len(paths))

	// Try to use batch document reader if available
	if batchReader, ok := idx.store.(store.BatchDocumentReader); ok {
		docs, err := batchReader.GetDocumentBatch(ctx, paths)
		if err != nil {
			return nil, fmt.Errorf("failed to batch get documents: %w", err)
		}

		for path, doc := range docs {
			if doc != nil {
				hashMap[path] = doc.Hash
			}
		}

		return hashMap, nil
	}

	// Fallback to individual queries
	for _, path := range paths {
		doc, err := idx.store.GetDocument(ctx, path)
		if err != nil {
			return nil, fmt.Errorf("failed to get document %s: %w", path, err)
		}

		if doc != nil {
			hashMap[path] = doc.Hash
		}
	}

	return hashMap, nil
}

// resultCollector collects chunking results and batches them for embedding
func (idx *Indexer) resultCollector(
	ctx context.Context,
	results <-chan fileResult,
	totalFiles int,
	batchSize int,
	onProgress ProgressCallback,
	stats *IndexStats,
	fileInfoMap map[string]FileInfo,
) error {
	currentBatch := chunkBatch{
		chunks:  make([]ChunkInfo, 0, batchSize),
		fileMap: make([]chunkFileMapping, 0, batchSize),
	}
	fileHashes := make(map[string]string)

	flushBatch := func() error {
		if len(currentBatch.chunks) == 0 {
			return nil
		}

		if err := idx.processBatch(ctx, currentBatch, fileHashes); err != nil {
			return err
		}

		// Reset batch
		currentBatch = chunkBatch{
			chunks:  make([]ChunkInfo, 0, batchSize),
			fileMap: make([]chunkFileMapping, 0, batchSize),
		}
		fileHashes = make(map[string]string)

		return nil
	}

	processedCount := 0
	for result := range results {
		// Report progress
		if onProgress != nil {
			onProgress(ProgressInfo{
				Current:     processedCount + 1,
				Total:       totalFiles,
				CurrentFile: result.path,
			})
		}
		processedCount++

		if result.err != nil {
			log.Printf("Error processing %s: %v", result.path, result.err)
			continue
		}

		if !result.needsIndex {
			// File unchanged, skip
			continue
		}

		// Add chunks to current batch
		fileInfo := fileInfoMap[result.path]
		fileHashes[result.path] = fileInfo.Hash

		for i, chunk := range result.chunks {
			currentBatch.chunks = append(currentBatch.chunks, chunk)
			currentBatch.fileMap = append(currentBatch.fileMap, chunkFileMapping{
				filePath: result.path,
				chunkIdx: i,
			})

			// Flush batch if size reached
			if len(currentBatch.chunks) >= batchSize {
				if err := flushBatch(); err != nil {
					return err
				}
			}
		}

		stats.FilesIndexed++
		stats.ChunksCreated += len(result.chunks)
	}

	// Flush remaining batch
	return flushBatch()
}
