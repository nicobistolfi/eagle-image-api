## What this changes

<!-- What does this do, and why is it needed? Link the issue it closes. -->

## How it was tested

<!-- Which tests cover it? Anything you verified by hand? -->

## Checklist

- [ ] `go test -race ./...` passes
- [ ] `gofmt -l .` prints nothing
- [ ] `go vet ./...`, `staticcheck ./...`, and `golangci-lint run ./...` pass
- [ ] New behaviour has tests; bug fixes have a test that fails without the fix
- [ ] Exported identifiers have doc comments
- [ ] README updated if a parameter, environment variable, or CLI flag changed
