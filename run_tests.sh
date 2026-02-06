#!/usr/bin/env bash
#
# Run all tests with recommended settings.
#
# Flags:
#   -race    Enables the Go race detector. This instruments memory accesses at
#            compile time so the runtime can detect unsynchronized reads/writes
#            across goroutines. Any data race causes the test to fail immediately
#            with a diagnostic trace. Essential for this project since the task
#            store and worker pool are heavily concurrent.
#
#   -count=1 Bypasses Go's test result cache. Without this, `go test` may skip
#            re-running tests whose source files haven't changed. Forcing count=1
#            ensures every test actually executes — important when chasing race
#            conditions that don't reproduce deterministically.

set -euo pipefail

go test -race -count=1 ./...
