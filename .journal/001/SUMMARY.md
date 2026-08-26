---
id: 001
title: First working session
date: 2026-08-25
status: complete
repos_touched: [incus-guest-agent]
related_sessions: []
---

## Goal
Establish what `incus-guest-agent` must build: a Talos Linux system extension
that runs the Incus guest agent inside Talos VMs on Incus, so
`incus-spire-attestor` (~/code/componere/incus-spire-attestor) can attest nodes
through `/dev/incus/sock`. De-risk the approach with a live spike, then produce
a reviewed architecture and an implementation plan.

## Outcome
Goal met. The spike proved the full chain end-to-end on real infrastructure
(Talos v1.13.9 VM on Incus 7.3, host `sandbox01`), the architecture was drafted
by a software-architect agent and simplified via an adversarial complexity
review, and a zero-drift implementation plan was produced. Implementation has
not started — that is the next session's work, beginning at PLAN.md Phase 1.

## Key Decisions
- CD-ROM (`agent:config` device) is the only v1 media source → Talos kernel has
  `CONFIG_NET_9P` unset (9p impossible); iso9660 is built in; virtiofs untested.
- Do not bundle `incus-agent` → the cdrom ships a static agent version-matched
  to the host, plus all TLS material (`agent.crt/key`, `server.crt`).
- Probe `/dev/sr*` instead of `/dev/disk/by-label/incus-agent` → Talos udev
  creates no label symlink for the cdrom.
- Wrapper owns preparation retry; machined (`restart: always`) owns post-start
  restart → no competing crash loops.
- No custom OCI assembler → pinned Sidero `bldr` first, `crane`/`regctl`
  fallback; melange/apko removed only after a hello-payload gate passes
  (complexity-review outcome).
- Two packages, four ports (`DeviceFinder`, `StageManager`, `AgentProcess`,
  `Waiter`); stage exactly five named files; fixed paths/retry as constants;
  no CLI/config/readiness surface (complexity-review outcome).
- Delivery order proves the two release blockers first: machined mount
  propagation and artifact-through-publisher (vertical spike, PLAN Phase 1).

## Changes
- `.gitignore` — added `ref/` for read-only reference clones (PR #6, squash-merged).
- `.journal/001/ARCHITECTURE.md` — approved architecture (journal branch only).
- `.journal/001/PLAN.md` — 4-phase implementation plan (journal branch only).
- No implementation code changed; repo is still the untouched template-go scaffold.

## Open Threads
- PLAN.md Phase 1 (vertical spike) is the next action: minimal wrapper + final
  service YAML + builder-selection gate + publisher dry-run + sandbox01 nonce
  proof.
- Builder choice (`bldr` vs `crane`/`regctl`) is an open gate in PLAN.md §2;
  a last-resort Go assembler requires explicit design-authority approval.
- virtiofs media source, Image Factory schematics, readiness surface: all
  deferred; each has a cheap prototype defined in ARCHITECTURE.md §7/§10 notes.
- sandbox01 `/tmp/talos-spike/` still holds reusable assets (Talos ISO,
  talosctl, kubectl, kubeconfig-era files); `/tmp` is self-cleaning.

## References
- `.journal/001/ARCHITECTURE.md` — design authority for implementation.
- `.journal/001/PLAN.md` — ordered implementation plan; start at Phase 1.
- `.journal/001/NOTES.md` — spike evidence (2026-08-25 23:05 entry has the full
  verified findings), agent/reviewer pipeline notes.
- PR: https://github.com/componere/incus-guest-agent/pull/6 (`ref/` gitignore).
- Reference clones (gitignored, re-clone if absent): `ref/extensions`
  (siderolabs/extensions — packaging pattern at guest-agents/qemu-guest-agent),
  `ref/incus` (lxc/incus — cmd/incus-agent, agent-loader scripts).
- Downstream consumer: ~/code/componere/incus-spire-attestor.
- Spike host: `sandbox01` (see ~/code/lab2/sandbox/README.md).

## Lessons
- The Incus `agent:config` cdrom carries the agent binary itself, not just
  config — host/guest version matching comes free; never bundle the agent.
- Talos PodSecurity blocks privileged pods in `default`; use `kube-system` for
  spike pods.
- The agent reports its mount-namespace OS to `incus info` (cosmetic; expected
  in the extension container too).
