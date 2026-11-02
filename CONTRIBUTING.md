# Contributing

## Development requirements

- Go 1.24 or newer compatible with `go.mod`.
- Linux is recommended for exercising host-detection behavior.
- Optional host tools such as `btrfs`, `snapper`, and `systemctl` are not required for unit tests.

## Checks

Run before submitting changes:

```bash
gofmt -w .
go test ./...
go vet ./...
go test -race ./...
go build ./cmd/eed
```

Tests must use mocks or fixtures for external commands and must not mutate the host. Snapshot creation, package transactions, service changes, and rollback must never run in ordinary tests.

## Safety and scope

Do not add functionality that reports success without verifying the result. Keep read-only diagnostics separate from modifying operations, use argument arrays rather than shell strings, and document unsupported behavior. Do not publish releases or upload user data as part of development.

The project is licensed under Apache-2.0; see [LICENSE](./LICENSE). Contributors should ensure they have the rights needed to submit their contributions under that license.
