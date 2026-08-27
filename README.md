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
documented `machine.pods` static pod is the only supported deployment.


## Runtime behavior

The wrapper:

1. checks `/dev/sr*` for valid Incus configuration media;
2. validates `incus-agent`, `agent.conf`, `agent.crt`, `agent.key`, and
   `server.crt`;
3. stages the files under `/var/run/incus-guest-agent/agent`;
4. starts exactly one host-supplied `incus-agent`; and
5. restarts through kubelet if the supervised process exits unexpectedly.

The pod remains observable through `talosctl containers --kubernetes` and
`talosctl logs --kubernetes` when kube-apiserver is unavailable.

## Development

[mise](https://mise.jdx.dev) installs the toolchain pinned in `mise.toml` and
`mise.lock`:

```sh
mise install
```

Moon is the task entry point:

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

