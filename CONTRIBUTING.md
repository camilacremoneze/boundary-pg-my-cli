# Contributing

Thank you for your interest in contributing to pgcli-boundary!

## Getting started

1. Fork the repository and clone your fork.
2. Copy `.env.example` to `.env` and fill in your Boundary details.
3. Install dependencies: `make deps`
4. Run tests to confirm everything works: `make test`

## Development workflow

```bash
make test   # run unit tests
make lint   # go vet + staticcheck
make fmt    # auto-format with gofmt
make build  # build the binary
make run    # build and launch the app
```

## Submitting changes

1. Create a feature branch from `main`.
2. Make your changes with clear, focused commits.
3. Add or update tests for any new behaviour.
4. Ensure `make test` and `make lint` pass with no errors.
5. Open a pull request describing what you changed and why.

## Reporting issues

Please open a GitHub issue with:
- A clear description of the problem or feature request.
- Steps to reproduce (for bugs).
- Your OS, Go version, and `boundary` CLI version.

## Code style

- Follow standard Go conventions (`gofmt`, `go vet`).
- Keep packages small and focused — the existing `internal/` structure reflects this.
- Avoid adding new dependencies unless strictly necessary.
