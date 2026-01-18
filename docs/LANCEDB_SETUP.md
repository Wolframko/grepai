# LanceDB Backend Setup

The LanceDB backend is an optional high-performance vector storage backend for grepai.

## Why LanceDB?

LanceDB offers:
- Fast vector search with HNSW indexing
- Columnar storage format (Apache Arrow)
- Embedded database (no separate server needed)
- Multi-version concurrency control (MVCC)

## Default Build (LanceDB Disabled)

By default, grepai builds **without** LanceDB support to avoid requiring CGO and native dependencies. If you try to use the `lancedb` backend without enabling it, you'll get an error:

```
LanceDB backend is not available. Rebuild with '-tags lancedb' to enable LanceDB support
```

## Enabling LanceDB Support

### Prerequisites

- Go 1.24+ with CGO enabled
- C compiler (clang on macOS, gcc on Linux)

### Step 1: Download LanceDB Native Libraries

```bash
# Download the artifact downloader script
curl -sSL https://raw.githubusercontent.com/lancedb/lancedb-go/main/scripts/download-artifacts.sh -o download-lancedb.sh
chmod +x download-lancedb.sh

# Run the script in your project root
./download-lancedb.sh

# This will create:
# - include/ directory with C headers
# - lib/ directory with native libraries
```

### Step 2: Set CGO Environment Variables

```bash
# For macOS and Linux
export CGO_ENABLED=1
export CGO_CFLAGS="-I$(pwd)/include"
export CGO_LDFLAGS="-L$(pwd)/lib"

# For permanent setup, add to your ~/.bashrc or ~/.zshrc:
echo 'export CGO_ENABLED=1' >> ~/.zshrc
```

### Step 3: Build with LanceDB Support

```bash
# Build with the lancedb build tag
go build -tags lancedb -ldflags "-s -w" -o bin/grepai ./cmd/grepai

# Or using make (update Makefile to include tags)
make build TAGS=lancedb
```

Alternatively, update the Makefile to always include LanceDB:

```makefile
build:
	go build -tags lancedb -ldflags "-s -w -X main.version=0.1.0" -o bin/grepai ./cmd/grepai
```

## Configuration

Once built with LanceDB support, configure it in `.grepai/config.yaml`:

```yaml
store:
  backend: lancedb
  lancedb:
    path: .grepai/lancedb  # Optional: defaults to .grepai/lancedb
```

## Troubleshooting

### "LanceDB backend is not available"

You need to rebuild with the `-tags lancedb` flag:

```bash
go build -tags lancedb -o bin/grepai ./cmd/grepai
```

### Undefined symbols during linking

Ensure you've downloaded the native libraries and set CGO_LDFLAGS:

```bash
export CGO_LDFLAGS="-L$(pwd)/lib"
```

### "permission denied" when downloading artifacts

Don't run the download script in `~/go/pkg/mod`. Run it in your project directory instead.

## Platform Support

LanceDB Go currently supports:

- **macOS**: arm64 (Apple Silicon), amd64 (Intel)
- **Linux**: arm64, amd64
- **Windows**: amd64

## Performance Comparison

| Backend   | Search Speed | Index Size | Setup Complexity |
|-----------|--------------|------------|------------------|
| GOB       | Moderate     | Small      | None             |
| Postgres  | Fast         | Medium     | Moderate         |
| LanceDB   | Very Fast    | Medium     | Moderate         |

## Alternative: Use GOB or Postgres

If you don't need LanceDB's performance benefits, stick with:

- **GOB** (default): Simple file-based storage, no dependencies
- **Postgres**: Production-ready with pgvector extension

Both work out of the box without additional setup.
