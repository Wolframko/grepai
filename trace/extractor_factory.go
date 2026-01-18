//go:build !treesitter

package trace

import "fmt"

// NewExtractor returns a symbol extractor based on the configured mode.
// Precise mode requires a build with the "treesitter" tag.
func NewExtractor(mode string) (SymbolExtractor, error) {
	switch mode {
	case "", "fast":
		return NewRegexExtractor(), nil
	case "precise":
		return nil, fmt.Errorf("precise mode requires building with the \"treesitter\" build tag")
	default:
		return nil, fmt.Errorf("unknown trace mode: %s", mode)
	}
}
