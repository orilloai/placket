package job

import (
	"github.com/orilloai/placket/core/receipts"
	"github.com/orilloai/placket/adapters/local"
	cli "github.com/orilloai/placket/cmd/placket"
	_ "github.com/orilloai/placket/adapters"
	_ "github.com/orilloai/placket/adaptersx"
	// "github.com/orilloai/placket/adapters/journal", in a comment, is not an import.
)

var _ = receipts.Verify
var _ = local.Call{}
var _ = cli.Main

const path = "github.com/orilloai/placket/adapters/cli"
