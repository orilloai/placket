// Package bad reaches out of the core into an adapter.
package bad

import (
	"github.com/orilloai/placket/adapters/local"
)

var _ = local.Server
