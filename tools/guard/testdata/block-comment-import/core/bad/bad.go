// Package bad hides an upward import behind a block comment whose second line starts with //.
package bad

import (
	/*
		// */"github.com/orilloai/placket/adapters/local"
)

var _ = local.Server
