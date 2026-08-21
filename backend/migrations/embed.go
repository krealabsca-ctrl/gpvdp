// Package migrations embebe los archivos SQL de golang-migrate en el binario.
package migrations

import "embed"

// FS contiene los archivos *.sql de migración (up/down).
//
//go:embed *.sql
var FS embed.FS
