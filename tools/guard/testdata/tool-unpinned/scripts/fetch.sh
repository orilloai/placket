#!/bin/sh
# go run golang.org/x/tools/cmd/stringer@latest, in a comment, is not read.
go install golang.org/x/tools/cmd/stringer
go run golang.org/x/tools/cmd/stringer@v0.30 -type T
go run ./scripts/local
(cd core && go run golang.org/x/tools/cmd/stringer@v0.30.0)
go run -mod=mod "golang.org/x/vuln/cmd/govulncheck@master" ./...
go install golang.org/x/tools/cmd/stringer@v0.0.0-20250101000000-0123456789ab
cargo run ./x
