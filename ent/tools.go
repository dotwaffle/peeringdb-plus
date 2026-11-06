//go:build tools

package ent

// Keep entc.go's entproto dependencies in the module graph during go mod tidy.
import _ "entgo.io/contrib/entproto"
