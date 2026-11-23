// Package migrations содержит SQL-миграции схемы MoloTalk.
package migrations

import "embed"

// Files — встроенные SQL-миграции. Каталог является единственным источником
// версий для рабочей команды migrate и интеграционных проверок.
//
//go:embed *.sql
var Files embed.FS
