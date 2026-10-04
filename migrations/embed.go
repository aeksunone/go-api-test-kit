// Package migrations exposes the initial schema for isolated test databases.
package migrations

import _ "embed"

//go:embed 001_init.sql
var Initial string
