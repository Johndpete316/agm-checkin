#!/usr/bin/env fish

set -x DATABASE_URL (grep DATABASE_URL .env | cut -d= -f2-)
set -x AUTH_PIN 1234
set -x ALLOWED_ORIGIN http://localhost:5173
go run ./bin/api
