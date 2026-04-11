#!/usr/bin/env fish

set -x DATABASE_URL (grep DATABASE_URL .env | cut -d= -f2-)
go run ./bin/seed/seed.go
