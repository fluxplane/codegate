package golang

import (
	"github.com/codewandler/codegate"
	"github.com/codewandler/codegate/internal/lang/goast"
)

type Config struct{}

func New(Config) codegate.Backend {
	return goast.New()
}
