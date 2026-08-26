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
