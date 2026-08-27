# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- Mission: run the host-supplied Incus guest agent in Talos VMs so
  `incus-spire-attestor` can attest through `/dev/incus/sock`. Current design
  authority: `.journal/002/ARCHITECTURE.md`; completed implementation plan:
  `.journal/002/PLAN.md`. Both session 001 design artifacts are superseded.
- Talos v1.13.9 extension services cannot run the real agent because their
  default seccomp profile rejects `AF_VSOCK`. The production path is a
  `machine.pods` static pod in `kube-system` with UID/GID 0, `privileged: true`,
  `hostNetwork: true`, and host `/dev`; separate live trials showed that each
  privilege reduction breaks required behavior.
- The `agent:config` CD-ROM ships the host-matched static agent and TLS material.
  Probe `/dev/sr*`, stage exactly five files transactionally to tmpfs under
  `/run/incus-guest-agent`, and execute the staged agent. Talos has no 9p support
  or `/dev/disk/by-label/incus-agent` symlink.
- The Go wrapper is PID 1 and a child subreaper. It supervises the agent process
  group, reaps descendants, and owns graceful shutdown with SIGKILL escalation.
- Deploy per node through Talos machine configuration with immutable image
  digests. Talos container and log APIs remain available without kube-apiserver.
- v0.1.0 is released. Image: `ghcr.io/componere/incus-guest-agent` (public,
  auto-linked to the repo, amd64+arm64). Releases flow through the pinned
  `meigma/release` reusable workflows on `v*` tags; release-please opens the
  release PR (`initial-version` 0.1.0, pre-major bump flags set).
- `gh attestation verify` on release artifacts requires
  `--signer-repo meigma/release` because provenance carries the reusable
  workflow identity; commands are documented in `SECURITY.md`.
- Live test host: `sandbox01` (Ubuntu, Zabbly Incus; see `~/code/lab2/sandbox`).
- Reference clones live in gitignored ref/ (siderolabs/extensions, lxc/incus);
  re-clone if missing.
