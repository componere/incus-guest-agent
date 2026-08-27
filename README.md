# incus-guest-agent

`incus-guest-agent` runs the host-supplied Incus guest agent inside Talos
Linux virtual machines. Talos is immutable and cannot run the loader that
normally installs the agent inside a guest, so without it `incus info`,
`incus exec`, and node-local `/dev/incus/sock` consumers do not work. This
project delivers the agent instead as a privileged Talos static pod that
discovers the Incus configuration medium, stages its five files into tmpfs,
and supervises the agent process.

The supported deployment is a digest-pinned `machine.pods` entry. It runs on
Linux `amd64` and `arm64`.

## Getting started

Attach the agent media on the Incus host:

```sh
incus config device add <instance> agent disk source=agent:config
```

Resolve a release to an immutable digest, render the static-pod patch from
[`deploy/talos/incus-guest-agent.yaml.tmpl`](deploy/talos/incus-guest-agent.yaml.tmpl),
and apply it to the Talos node without a reboot:

```sh
DIGEST="$(docker buildx imagetools inspect \
  ghcr.io/componere/incus-guest-agent:<version> --format '{{.Manifest.Digest}}')"
sed "s|sha256:<digest>|$DIGEST|" \
  deploy/talos/incus-guest-agent.yaml.tmpl > incus-guest-agent.yaml

talosctl --nodes <node> patch machineconfig --mode no-reboot \
  --patch @incus-guest-agent.yaml
```

For prerequisites, node-by-node rollout, and the verification steps, follow
[Install on Talos under Incus](https://componere.github.io/incus-guest-agent/how-to/install/).

## Documentation

The full documentation lives at
[componere.github.io/incus-guest-agent](https://componere.github.io/incus-guest-agent/):

- [Install on Talos under Incus](https://componere.github.io/incus-guest-agent/how-to/install/)
- [Update and roll back](https://componere.github.io/incus-guest-agent/how-to/update-rollback/)
- [Runtime and operations reference](https://componere.github.io/incus-guest-agent/reference/runtime/)
- [Why the static pod is privileged](https://componere.github.io/incus-guest-agent/explanation/privileged-static-pod/)

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

