module github.com/orilloai/placket

go 1.27

// The core, from this tree until it is first tagged.
require (
	github.com/orilloai/placket/core v0.0.0
)

replace github.com/orilloai/placket/core => ./core
