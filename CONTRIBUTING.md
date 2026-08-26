# Contributing

Use [SECURITY.md](SECURITY.md) for vulnerabilities. Report non-security bugs
through GitHub issues.

## Pull requests

Keep each change focused on one problem. When behavior changes:

1. add or update tests for the observable contract;
2. update operator documentation;
3. use a Conventional Commit subject; and
4. run `moon run root:check` before requesting review.

The architecture keeps orchestration in `internal/agent` and Linux I/O in
`internal/linux`. Put new side effects behind consumer-owned ports and generate
adapter mocks with Mockery; do not write mocks by hand.

## Local setup

Install the toolchain pinned by mise:

```sh
mise install
```

Run the standard checks:

```sh
moon run root:format
moon run root:lint
moon run root:build
moon run root:test
moon run root:check
```

Run the command directly:

```sh
go run ./cmd/incus-guest-agent --version
```

## Release changes

Release Please derives release notes and version changes from Conventional
Commit subjects. Do not create release tags manually or enable a publisher to
work around a failed validation gate.

