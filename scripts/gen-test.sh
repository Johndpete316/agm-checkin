#!/bin/bash
# Usage: ./gen-tests.sh ./internal/auth

PACKAGE=${1:-./...}

claude -p "
Generate unit tests and security regression tests for all Go files in $PACKAGE.

Work through this loop until complete:
1. Read all .go source files in $PACKAGE (skip existing _test.go files)
2. Generate corresponding _test.go files
3. Run: go build $PACKAGE
4. Run: go test -race $PACKAGE
5. Run: go vet $PACKAGE
6. If any command fails, fix the errors and return to step 3
7. Only stop when all three commands exit 0

Security tests must use the TestSecurity_ prefix.
Use table-driven tests. Mock external deps with interfaces.
" \
  --allowedTools "Read,Write,Edit,Bash" \
  --max-turns 50
