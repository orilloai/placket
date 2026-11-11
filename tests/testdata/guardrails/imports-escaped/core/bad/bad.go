// Package bad reaches an adapter through an import path written with an escape.
package bad

import (
	_ "github.com/orilloai/placket/\x61dapters/local"
)
