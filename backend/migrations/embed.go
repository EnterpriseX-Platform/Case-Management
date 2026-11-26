// Package migrations embeds the SQL schema, applied in order at start.
package migrations

import _ "embed"

//go:embed 001_init.sql
var Init string

//go:embed 002_ops.sql
var Ops string

//go:embed 003_work.sql
var Work string

// All returns the migrations in order. Every script is idempotent.
func All() []string { return []string{Init, Ops, Work} }
