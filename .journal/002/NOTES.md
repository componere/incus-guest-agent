---
id: 002
title: Review preview artifacts
started: 2026-08-26
---

## 2026-08-26 09:53 — Kickoff
Goal for the session: Review the artifacts generated in the preview session.
Current state of the world: Session 001 completed a live Talos/Incus spike and produced `.journal/001/ARCHITECTURE.md` and `.journal/001/PLAN.md`; implementation has not started.
Plan: Inspect both artifacts against the repository, reference implementations, and recorded spike evidence; report concrete findings and next actions.

## 2026-08-26 09:58 — Preview artifact review
Reviewed `.journal/001/ARCHITECTURE.md` and `.journal/001/PLAN.md` against the current template, `ref/extensions`, `ref/incus`, and the repository operating preferences.

Findings:

1. The runtime design has no abnormal-exit recovery for the shared media/tmpfs mounts. Both mounts propagate through `/run`; a SIGKILL or crash can leave stale mounts that block the `restart: always` path. Incus' own Linux loader defensively runs `umount -l /run/incus_agent` before mounting a fresh tmpfs. Startup reconciliation and a forced-restart scenario are required.
2. PID 1 supervision is underspecified. The wrapper only waits for and signals the direct `incus-agent` child, while `incus-agent` starts command subprocesses for guest exec. The design must define process-group shutdown and orphan reaping, or explicitly prove machined provides those semantics.
3. Phase 1's builder gate cannot run as written. `bldr` requires a root `Pkgfile` and package `pkg.yaml`, but the plan excludes those files pending design approval. It also requires a separately pinned `extensions-validator`; that project has no releases and no mise registry entry. The official extensions tree injects `ghcr.io/siderolabs/extensions-validator:<revision>` through `internal/base/pkg.yaml` and runs validation inside the bldr build.
4. The live commands use the wrong Talos service ID. Extension services are exposed as `ext-<name>`; the reference QEMU extension uses `ext-qemu-guest-agent`. The expected ID is `ext-incus-guest-agent`, not `incus-guest-agent`.
5. `PLAN.md` is a 662-line zero-design-drift waterfall plan with mandatory sequential gates and repeated approval stops. That conflicts with the repository's agile operating preference and will cause rework before the unproven builder/mount details are learned.

Decision: preserve the architecture's validated product choices—host-supplied agent, CD-ROM discovery, `/dev/sr*` probing, scratch extension, and blocker-first live proof—but do not treat either artifact as approved design authority yet. Replace the implementation plan with a short Phase 1 experiment: a real `Pkgfile`/`pkg.yaml`, minimal wrapper, validator-in-build, disposable OCI publication, and live checks covering hot-add, nonce, normal restart, forced kill/restart, stale-mount cleanup, and PID 1 behavior. Refine the production architecture only after those results.

## 2026-08-26 11:07 — Runtime spike exposes Talos seccomp blocker
Implemented and ran the proposed experiment without modifying `.journal/001/ARCHITECTURE.md` or `.journal/001/PLAN.md`. The spike is preserved on `spike/runtime` at commit `491f43c`.

Build evidence:

- Added a real bldr `Pkgfile`, `internal/base/pkg.yaml`, and `extension/incus-guest-agent/pkg.yaml`. The base pins `ghcr.io/siderolabs/tools:v1.15.0-alpha.0-2-gd102f15` and `ghcr.io/siderolabs/extensions-validator:84cecb0`.
- bldr v0.6.3 built both `linux/amd64` and `linux/arm64`. Each architecture ran the wrapper's `--version` command and `extensions-validator` inside the package test step.
- Published disposable multi-platform extension `ttl.sh/componere-incus-guest-agent-spike-20260826-3:24h@sha256:a170f6937d43811bf0b4703642270dfbcf3a97f2618d774bf4cb5c78266ce08f`, embedded it into a Talos v1.13.9 installer with `imager`, and installed it on Incus 7.3 VM `talos-agent-wrapper-spike` on `sandbox01`.
- The validator accepted an entrypoint that was absolute in the host extension tree but absent from the service container root. Live startup exposed this; changing it to container-root-relative `./incus-guest-agent` made `ext-incus-guest-agent` run. Validator success is necessary but does not prove service startup.

Live results:

1. Hot-add works. The service started before media existed, waited on `/dev/sr*`, detected the newly attached Incus `agent:config` CD-ROM as `/dev/sr0`, validated and staged all five files into tmpfs, and launched the host-supplied binary.
2. The real Incus agent cannot run in a Talos extension-service container. It exits immediately with `listen vsock vm(4294967295):8443: socket: operation not permitted`; the nonce round trip therefore cannot complete. `/proc/<wrapper>/status` showed `Seccomp: 2` and one active filter despite all grantable capabilities. Talos v1.13.9 hard-codes its extension-service OCI policy and the service YAML `Security` schema has no seccomp override. The earlier privileged Kubernetes pod succeeded because that execution environment was not subject to this filter.
3. PID 1 supervision works in the wrapper. Its host status reported `NSpid: <host-pid> 1`. A fake static agent forked an orphaned grandchild; the wrapper adopted and reaped it, logging `reaped orphaned agent descendant`.
4. Graceful restart works. `talosctl service ext-incus-guest-agent restart` sent SIGTERM, the fake agent observed `terminated`, the wrapper exited cleanly, and Talos started a new wrapper and staged the media again.
5. Forced restart works. A Talos debug container sent SIGKILL to the wrapper host PID. No termination message was emitted; Talos observed the task failure, restarted the service with a new host PID, and the wrapper staged and ran the fake agent again.
6. The stale-host-mount premise is false for the tested service configuration. The staging tmpfs exists only in the extension container's mount namespace: neither `/run/incus-guest-agent/agent` nor its marker files appeared on the Talos host, and `talosctl mounts` showed no propagated staging mount. A SIGKILL destroys that namespace, so the restarted container gets no stale tmpfs. Outward propagation would require `security.rootfsPropagation: shared`; the spike does not set it because the agent does not need host visibility for staging.

Decision: the bldr and wrapper mechanics are viable, but the current extension-service architecture is blocked by Talos' seccomp filter on `AF_VSOCK`. Do not proceed to production implementation or revise the stored architecture/plan until choosing and proving an execution model that permits the host-supplied agent's vsock listener.

Teardown complete: deleted `talos-agent-wrapper-spike`, removed its generated remote/local media and installer artifacts, and removed the temporary buildx builder. The pushed `spike/runtime` branch and 24-hour OCI references remain as the durable and short-lived evidence respectively.
