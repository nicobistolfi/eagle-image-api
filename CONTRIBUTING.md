# Contributing to Eagle Image API

Thanks for taking the time to contribute. This document covers how to get a
development environment running, what the project expects from a change, and
how a pull request gets reviewed.

By participating you agree to the [Code of Conduct](CODE_OF_CONDUCT.md).

## Getting set up

Eagle wraps [libvips](https://www.libvips.org/) through
[govips](https://github.com/davidbyttow/govips), so cgo and a local libvips
install are required — a plain `go build` fails without them.

```bash
# macOS
brew install vips pkg-config

# Debian / Ubuntu
sudo apt-get install -y libvips-dev pkg-config
```

Then clone and verify the toolchain:

```bash
git clone https://github.com/nicobistolfi/eagle-image-api.git
cd eagle-image-api
go build ./...
go test ./...
```

The repository uses [Task](https://taskfile.dev) as its task runner. `task
--list` shows the available targets; `task test` and `task build` are the two
you will use most.

## Making a change

1. Open an issue first for anything larger than a bug fix or a small
   improvement. Agreeing on the approach before code is written saves
   everyone time.
2. Branch off `main`. Name the branch after what it does, e.g.
   `fix/avif-timeout` or `feat/webp-effort-flag`.
3. Keep the change focused. Unrelated refactors belong in their own pull
   request, where they can be reviewed on their own merits.

## What a change needs

Before opening a pull request, make sure all of the following pass. CI runs
exactly these checks, so a green local run means a green pipeline.

```bash
go build ./...
go test -race ./...
go vet ./...
gofmt -l .              # must print nothing
staticcheck ./...
golangci-lint run ./...
```

Beyond the tooling:

- **Tests.** New behaviour needs tests, and bug fixes need a test that fails
  without the fix. The project holds statement coverage at 80% or above; the
  Codecov check on your pull request reports where you landed.
- **Documentation comments.** Every exported identifier needs a doc comment
  starting with its name, because these are published on
  [pkg.go.dev](https://pkg.go.dev/github.com/nicobistolfi/eagle-image-api).
- **Errors.** Wrap with `fmt.Errorf("doing the thing: %w", err)` so the
  failure path reads as a sentence. Do not discard errors silently.
- **README.** Update it when you add or change a query parameter, an
  environment variable, or a CLI flag.

### Testing against libvips

Image tests run against real libvips rather than a mock, and fixtures are
generated with the standard library's `image/png` encoder instead of embedded
blobs. libvips decodes lazily, so a malformed fixture will not fail on load —
it fails much later, during export, with a confusing error. Generate real
pixel data.

If your libvips build lacks an encoder (AVIF is the usual one), the affected
tests skip rather than fail.

## Commit messages

Write a short imperative summary line, then a body explaining *why* the change
is needed if that is not obvious from the summary. Reference the issue the
change closes.

```
Fix AVIF encoding timeout on large images

Images above the configured megapixel ceiling took longer to encode than the
Lambda timeout allowed, so the request failed instead of falling back. Skip
AVIF for those and serve WebP.

Closes #42
```

## Pull requests

Open the pull request against `main` and fill in what changed and why. A
maintainer reviews it, and CI must be green before it can merge. Expect review
comments — they are about the code, not about you.

Pull requests are squash-merged, so the pull request title becomes the commit
message on `main`. Give it the same care as a commit summary.

## Releases

Releases are cut by maintainers by pushing a SemVer tag:

```bash
git tag -a v1.2.3 -m "v1.2.3"
git push origin v1.2.3
```

This triggers GoReleaser, which builds the `eagle` CLI for macOS and Linux,
publishes the archives to the GitHub release, and updates the Homebrew tap.

## Reporting security issues

Do not open a public issue for a security vulnerability. Follow the process in
[SECURITY.md](SECURITY.md).
