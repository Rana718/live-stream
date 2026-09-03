//go:build tools

package tools

// buf is installed via `make install` (go install .../buf@$(BUF_VERSION)) — it
// is deliberately NOT a tool-dependency here to keep its large module graph out
// of go.mod. sqlc is likewise installed as a pinned binary.
import (
	_ "github.com/swaggo/swag"
)
