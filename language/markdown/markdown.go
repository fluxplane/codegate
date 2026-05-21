package markdown

import (
	"github.com/fluxplane/codegate"
	internalmarkdown "github.com/fluxplane/codegate/internal/lang/markdown"
)

// Config controls the Markdown backend. It is intentionally empty for the
// initial backend release.
type Config struct{}

// New returns the built-in Markdown backend.
func New(Config) codegate.Backend {
	return internalmarkdown.New()
}
