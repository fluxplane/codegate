package golang

import (
	"github.com/fluxplane/codegate"
	"github.com/fluxplane/codegate/internal/lang/goast"
)

// Config controls the Go backend. It is intentionally empty for the initial
// backend release.
type Config struct{}

// New returns the built-in Go backend.
func New(Config) codegate.Backend {
	return goast.New()
}
