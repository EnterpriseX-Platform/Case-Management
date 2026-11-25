// Package migrations embeds the SQL schema.
package migrations

import _ "embed"

//go:embed 001_init.sql
var Init string
