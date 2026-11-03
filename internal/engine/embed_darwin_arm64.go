package engine

import _ "embed"

//go:embed bin/core-darwin-arm64
var darwinARM64 []byte

func init() { embedded = darwinARM64 }
