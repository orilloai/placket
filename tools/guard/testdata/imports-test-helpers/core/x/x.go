// Package x is built into the core, so it may not import a test helper.
package x

import "github.com/orilloai/placket/core/internal/testing/h"

// Read reaches the world through a test helper.
var Read = h.Read
