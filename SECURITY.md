# Security Policy

## Supported versions

Security fixes are applied to the latest released version. Please upgrade to
the most recent release before reporting an issue.

| Version | Supported |
| ------- | --------- |
| Latest release | Yes |
| Older releases | No |

## Reporting a vulnerability

Please do not open a public issue for a security vulnerability.

Report it through GitHub's private reporting form at
[Security > Report a vulnerability](https://github.com/nicobistolfi/eagle-image-api/security/advisories/new),
which opens a private advisory visible only to the maintainers.

Include what you can:

- The version or commit affected
- What an attacker can do with it
- Steps to reproduce, ideally a request that demonstrates the problem
- Any suggested fix

You can expect an acknowledgement within a week. Once a fix is ready it ships
in a patch release, and the advisory is published crediting you unless you
prefer otherwise.

## Scope

Eagle fetches images from URLs supplied in the request, so the areas most
worth scrutiny are:

- The origin allowlist (`ORIGIN_WHITELIST`) and any way to bypass it
- Server-side request forgery through the `url` parameter
- Resource exhaustion via crafted images or transformation parameters
- Anything that lets a request reach the AWS credentials the Lambda runs with

Reports about a deployment's own misconfiguration — for example running with
`ORIGIN_WHITELIST=*` on a private network — are worth discussing in a public
issue instead, since the fix is documentation rather than code.
