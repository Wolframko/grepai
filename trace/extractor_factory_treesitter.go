//go:build treesitter

package trace

import "fmt"

// NewExtractor returns a symbol extractor based on the configured mode.
func NewExtractor(mode string) (SymbolExtractor, error) {
	switch mode {
	case "", "fast":
		return NewRegexExtractor(), nil
	case "precise":
		return NewTreeSitterExtractor()
	default:
		return nil, fmt.Errorf("unknown trace mode: %s", mode)
	}
}
