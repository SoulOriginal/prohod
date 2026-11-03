package engine

import _ "embed"

//go:embed bin/core-linux-amd64
var linuxAMD64 []byte

func init() { embedded = linuxAMD64 }
