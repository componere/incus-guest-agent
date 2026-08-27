# incus-guest-agent

`incus-guest-agent` runs the host-supplied Incus guest agent inside Talos
virtual machines. A privileged Talos static pod discovers the Incus
configuration medium, copies its five required files into tmpfs, starts the
agent, forwards shutdown signals, and reaps descendant processes.

The supported deployment is a digest-pinned `machine.pods` entry. It runs on
Linux `amd64` and `arm64`.

## Documentation

- [Install on Talos under Incus](docs/docs/how-to/install.md)
- [Update and roll back](docs/docs/how-to/update-rollback.md)
- [Runtime and operations reference](docs/docs/reference/runtime.md)
- [Why the static pod is privileged](docs/docs/explanation/privileged-static-pod.md)

The canonical patch template is
[`deploy/talos/incus-guest-agent.yaml.tmpl`](deploy/talos/incus-guest-agent.yaml.tmpl).
Resolve a stable release to an immutable OCI digest before applying it. The
documented `machine.pods` static pod is the only supported deployment. Runtime
paths, the startup sequence, and log messages are in the
[runtime reference](docs/docs/reference/runtime.md).

## Development

[mise](https://mise.jdx.dev) installs the toolchain pinned in `mise.toml` and
`mise.lock`:

```sh
mise install
```

Moon is the task entry point; `root:check` runs format, lint, build, test, and
the docs build:

```sh
moon run root:check
```

Run the command directly:

```sh
go run ./cmd/incus-guest-agent --version
```

[CONTRIBUTING.md](CONTRIBUTING.md) lists the individual tasks and the
architecture rules for changes.

Build the host-architecture OCI image locally:

```sh
mise run image-local
docker run --rm incus-guest-agent:dev --version
```

The release pipeline builds static Linux binaries, packages them into signed
Wolfi APKs with Melange, and assembles the multi-architecture image with apko.
Published release assets include checksums, SBOMs, signatures, and provenance.

## Contributing

See [CONTRIBUTING.md](CONTRIBUTING.md).

## Security

See [SECURITY.md](SECURITY.md) for the private vulnerability reporting path.

