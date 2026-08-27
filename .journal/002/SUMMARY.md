---
id: 002
title: Review preview artifacts
date: 2026-08-26
status: complete
repos_touched: [incus-guest-agent]
related_sessions: [001]
---

## Goal

Review the architecture and implementation plan from session 001 against the
repository, upstream references, and live evidence. Correct unsupported design
assumptions before implementation.

## Outcome

Goal met and expanded through implementation. The review found that Talos
extension services block the Incus agent's `AF_VSOCK` listener. A disposable
static-pod experiment proved the replacement execution model. The architecture
and plan were revised from that evidence, the complete product implementation
was squash-merged through PR #8, and the repository configuration was corrected
through PRs #10 and #11.

The production image and Talos `machine.pods` deployment passed the full live
matrix on Talos v1.13.9 and Incus 7.3. Local checks and required GitHub checks
passed before merge. The local `master` branch is current and clean. Merged
feature worktrees and remote branches were removed. The two spike worktrees were
removed while their branches remain as read-only experimental evidence.

## Key Decisions

- Run the agent wrapper as a privileged `machine.pods` static pod in
  `kube-system`. Do not use a Talos extension service: its default seccomp
  profile rejects the agent's `AF_VSOCK` listener.
- Keep the proven runtime profile: UID/GID 0, `privileged: true`,
  `hostNetwork: true`, and host `/dev`. Separate live trials showed that each
  privilege reduction breaks required behavior.
- Keep `agent:config` as the only media source. Probe `/dev/sr*`, stage the
  host-matched binary and four TLS/config files into tmpfs, and execute the
  staged binary.
- Let the Go wrapper run as PID 1, become a child subreaper, supervise the
  process group, reap descendants, and own graceful shutdown with SIGKILL
  escalation.
- Publish a minimal Wolfi image through Melange and apko for `linux/amd64` and
  `linux/arm64`. The image contains no shell, package manager, or bundled
  `incus-agent`.
- Deploy per node through Talos machine configuration with immutable image
  digests. Use Talos container and log APIs for observability when the
  Kubernetes API is unavailable.
- Keep the repository as a product repository, not a GitHub template. Protect
  tags with the installed `componere-release` GitHub App and repository
  administrators as bypass actors.

## Changes

- `.journal/002/ARCHITECTURE.md` — evidence-backed static-pod architecture;
  supersedes the session 001 architecture.
- `.journal/002/PLAN.md` — outcome-oriented implementation plan; completed.
- Go application — command, orchestration ports, Linux device and staging
  adapters, process supervision, generated mocks, and behavior-focused tests.
- `deploy/talos/incus-guest-agent.yaml.tmpl` — canonical digest-pinned Talos
  static-pod patch.
- `melange.yaml`, `apko.yaml`, and `.goreleaser.yaml` — two-architecture static
  binary, package, and minimal image pipeline.
- GitHub workflows — canonical release inputs, production publication, and
  scanning of the real Melange/apko image.
- `README.md`, `SECURITY.md`, `CONTRIBUTING.md`, and `docs/` — product identity,
  installation, update and rollback, runtime reference, privilege rationale,
  and release prerequisites.
- `.github/repository-settings.toml` — non-template repository state and the
  installed release-app actor.

## Verification

- Deterministic Mockery regeneration, repository-settings Python tests, and
  `mise exec -- moon run root:check --summary minimal` passed.
- Linux amd64 and arm64 process-supervision tests and Linux golangci-lint
  v2.12.2 passed in containers.
- GoReleaser snapshot and no-publish release rehearsal passed.
- Published image inspection proved exactly `linux/amd64` and `linux/arm64`,
  nonroot image metadata, the expected entrypoint, and no shell, package manager,
  or template binary.
- A fresh Talos VM passed cold boot, hot media attachment, guest reporting,
  nonce round trips, forced recovery, image update, API outage, media removal and
  reattachment, reboot persistence, and digest rollback.
- PR #8 passed `ci` and GitHub Pages before squash merge as
  `b6e0c278967f8209d136a55dce816b58ee90ddf8`.
- Repository configuration converged with no supported changes required. Direct
  API checks confirmed both managed rulesets and the non-template state.

## Unexpected Findings

- `extensions-validator` accepts package structure but does not prove service
  startup. The first live service run exposed an incorrect entrypoint and then
  Talos's non-configurable extension-service seccomp restriction.
- Static pods bypassed that restriction and remained observable through Talos
  while kube-apiserver was paused.
- The first disposable publication exposed a GHCR visibility prerequisite:
  newly created packages must be linked to the repository and made public before
  anonymous digest pulls work.
- Darwin checks did not load Linux-only lint findings. PR CI found them, and
  Linux container checks now cover the process-supervision code.
- GitHub did not attach new check suites to corrected PR #7. PR #8 replaced it
  at the same corrected branch head.
- The repository manifest still declared template status and a release app that
  is not installed. Applying it without review would have changed the product
  repository back into a template and failed tag-rule creation.

## Open Threads

- Release PR #9 is open with passing checks. Review and merge it separately when
  a `1.0.0` publication is intended.
- The first real GHCR publication must link the package to this repository, make
  it public, and verify anonymous digest resolution before deployment.
- Dependabot reports one high-severity ReDoS advisory and one medium-severity
  path-traversal advisory for `pymdown-extensions` in `docs/uv.lock`.
- The repository configuration script still reports the manual settings that
  GitHub does not expose through documented repository-level REST APIs.

## References

- `.journal/002/ARCHITECTURE.md` — current design authority.
- `.journal/002/PLAN.md` — completed implementation plan.
- `.journal/002/NOTES.md` — review, spike, release, deployment, and repository
  configuration evidence.
- PR #8: https://github.com/componere/incus-guest-agent/pull/8
- PR #10: https://github.com/componere/incus-guest-agent/pull/10
- PR #11: https://github.com/componere/incus-guest-agent/pull/11
- Release PR #9: https://github.com/componere/incus-guest-agent/pull/9
- Read-only spike branches: `spike/runtime` at `491f43c` and
  `spike/static-pod` at `33f93b2`.
- Live test host: `sandbox01` (see `~/code/lab2/sandbox`).
