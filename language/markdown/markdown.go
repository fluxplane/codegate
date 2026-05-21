package markdown

import (
	"github.com/codewandler/codegate"
	internalmarkdown "github.com/codewandler/codegate/internal/lang/markdown"
)

type Config struct{}

func New(Config) codegate.Backend {
	return internalmarkdown.New()
}
