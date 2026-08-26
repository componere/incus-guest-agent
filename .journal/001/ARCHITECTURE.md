# Architecture: `incus-guest-agent` Talos system extension

Status: proposal for review — produced by a software-architect agent from the
session-001 spike results, then simplified per an adversarial complexity
review. Spike evidence and rejected paths are in `NOTES.md` (session 001).

## 1. What this is

A small Linux PID 1 wrapper packaged as a Talos system extension. It discovers
the Incus `agent:config` CD-ROM, stages the host-supplied files into tmpfs, and
runs the host-shipped `incus-agent` from there. The v1 contract is exactly one
thing: **`/dev/incus/sock` works inside the Talos VM**, so consumers like
`incus-spire-attestor` can read claims and the challenge nonce.

Non-negotiable decisions carried from the spike:

- **No bundled `incus-agent`.** The CD-ROM ships a static agent version-matched
  to the host. Bundling would create version skew and a second release
  lifecycle.
- **CD-ROM only.** 9p is absent from the Talos kernel; virtiofs is untested.
  Probe `/dev/sr*` — Talos creates no `/dev/disk/by-label/incus-agent` symlink.
- **Static Go binary, scratch payload.** No shell, no base image, no packages.
- **Fixed paths and policy.** No config file, no flags beyond `--version`
  (release convention), no Cobra/Viper, no readiness endpoint, no metrics.
- **Raw OCI image reference** is the documented v1 install path
  (`machine.install.extensions`). Image Factory deferred.

Known v1 limitation: the agent reports the extension container's mount-ns view
to `incus info` (cosmetic), and host-side `incus exec` sees the minimal
container rootfs, not the Talos host. Acceptable — the socket consumer is the
product.

## 2. Decomposition

```text
cmd/incus-guest-agent/main.go       composition root, signals, --version
internal/agent/                     core state machine, no side effects (A1)
  doc.go, agent.go, ports.go
  mocks/                            mockery-generated (T2/T3 rules)
internal/linux/                     Linux adapters (device, stage, process)
  doc.go, stage.go, process.go
extension/
  manifest.yaml.tmpl                Talos extension manifest (version rendered)
  incus-guest-agent.yaml            machined service spec
```

Four consumer-owned ports, declared in `internal/agent`:

| Port | Contract |
|---|---|
| `DeviceFinder` | Sorted list of `DevicePath` candidates matching `/dev/sr*`. No mounting. |
| `StageManager` | One transaction: mount candidate ro-iso9660 → validate → copy the five files into a fresh 0700/50M tmpfs at `/run/incus_agent` → unmount media. Owns cleanup of its mounts. |
| `AgentProcess` | Start the staged binary (CWD `/run/incus_agent`, inherited stdio), forward signals, report exit. |
| `Waiter` | Cancelable delay for retry (deterministic tests). |

`DevicePath` is the one typed domain term (I1) — it crosses the core boundary.
Paths, tmpfs options, and retry limits are compile-time constants. Errors are
ordinary wrapped errors; at most one narrow typed error distinguishes
"child exited after successful start" from "preparation failed". No `Attempt`
objects, no error-aggregation framework, no static-config validation of
constants.

Placeholder template code (`cmd/template-go`, `internal/{cli,config,templateinfo}`)
is removed, not shimmed.

## 3. Runtime behavior

Fixed contract (all values from the verified spike):

| Item | Value |
|---|---|
| Probe glob | `/dev/sr*`, lexically sorted, block devices only |
| Probe mount | `/run/incus-guest-agent/media`, ro, `iso9660`, `nosuid,nodev,noexec` |
| Stage | tmpfs `/run/incus_agent`, `mode=0700,size=50M` |
| Staged files | exactly `incus-agent`, `agent.conf`, `agent.crt`, `agent.key`, `server.crt` |
| Exec | `./incus-agent`, CWD `/run/incus_agent`, no args, no shell |
| Retry | 1, 2, 4, 8, 16, then 30s cap; no jitter; forever until canceled |

Loop: enumerate candidates → for each, mount and require the five regular
files (agent must have exec bit) → first valid candidate is staged and the
media unmounted → start the child → wait. Copies are streamed (P2), preserve
permission bits only; no recursion, no symlink handling, no full-tree
re-validation — the five checked copies *are* the validation. A post-stage
media unmount failure is logged and retried at shutdown, never blocks the
agent start.

Failures: any preparation failure tears down owned mounts and re-enters the
capped retry loop (E3 — covers absent media, hot-added drive, transient mount
or copy errors). One warning per failed sweep; per-candidate misses at debug.
Owned directories are created once at startup.

Supervision: the wrapper stays PID 1, the agent is its direct child. SIGTERM /
SIGINT forward to the child; the wrapper waits, cleans owned mounts, exits 0.
If the child exits on its own, the wrapper cleans up and exits non-zero —
**machined's `restart: always` owns restart**, giving a fresh re-probe and
re-stage. No internal crash loop; `syscall.Exec` rejected (loses reaping,
signal forwarding, cleanup).

## 4. machined service spec

```yaml
name: incus-guest-agent
depends:
  - path: /system/run/machined/machine.sock
  - path: /dev/vsock
container:
  entrypoint: ./incus-guest-agent
  mounts:
    - { source: /dev, destination: /dev, type: bind, options: [rshared, rbind, rw] }
    - { source: /run, destination: /run, type: bind, options: [rshared, rbind, rw] }
restart: always
```

machined owns: prerequisites, container lifecycle, logs, restart. Wrapper
owns: discovery, staging, retry, signal forwarding. Deliberately **no
`/dev/sr0` dependency** — the drive number varies and hot-add must be
recoverable by the retry loop, not blocked at service start.

`/dev` rw so the agent can create `/dev/incus/sock`; `/run` rw + rshared for
the tmpfs stage. Whether these propagation settings behave as assumed inside a
machined container is **release-blocking and unproven** — hence the delivery
order below.

## 5. Extension payload and build

Per-platform image content (nothing else):

```text
/manifest.yaml
/rootfs/usr/local/lib/containers/incus-guest-agent/incus-guest-agent   0755
/rootfs/usr/local/etc/containers/incus-guest-agent.yaml                0644
```

Manifest: `version: v1alpha1`, name/author/description, compatibility
`talos: ">= v1.13.9"` — the tested line. Widen only with evidence.

**Build recommendation (revised by complexity review):** do not write a custom
OCI assembler. Prefer, in order:

1. **Pinned Sidero `bldr`** — the extension ecosystem's own builder — invoked
   from a Moon task, pinned in mise; output feeds the existing publish /
   sign / SBOM / provenance workflow unchanged.
2. If `bldr` can't produce the publisher's required artifact shape: a short
   pinned `crane`/`regctl` composition.
3. A Go `go-containerregistry` assembler is the last resort, not the default.

melange/apko don't fit (they'd wrap two files in an APK plus a Wolfi root the
extension must not contain) but are **removed only after** the chosen builder
passes the hello-payload gate: `extensions-validator` OK on both platforms +
existing publisher dry-run OK.

Release gates, one job each: builder creates the image → `extensions-validator`
checks each platform payload → one index assertion (amd64+arm64) only if the
validator doesn't cover it → one vulnerability scan of the final image (or the
binary if scanners can't see into scratch) → existing signing/SBOM/provenance/
release-please flow unchanged. GoReleaser trims to static linux amd64/arm64
with version injection; release-please stays the single version source,
rendered into `manifest.yaml` and OCI annotations.

## 6. Test strategy (T1–T3)

- **T1 (unit):** retry cap + cancellation policy; five-file staging helper
  against temp dirs (missing file, non-regular entry, exec bit, copy error).
- **T2 (mockery mocks of the four ports):** four behaviors — no media retries
  until canceled; invalid first candidate cleaned before the next; preparation
  or start failure retries without a partial start; success uses the fixed CWD
  and distinguishes child exit from requested shutdown. Order asserted only
  for safety invariants (cleanup before next candidate; never start after
  failed stage). Adapter tests: process signal-forwarding with a helper child;
  filesystem behavior in temp dirs. No pretend-mount fixtures — privileged
  mount truth lives in T3.
- **T3 (e2e):** `extensions-validator` per platform on every image build;
  live loop on `sandbox01` (manual first, Diátaxis runbook): boot disposable
  Talos VM without `agent:config` → service must sit in retry; hot-add the
  device → `/dev/incus/sock` appears without service restart; nonce round-trip
  via `user.*` config; repeat nonce after `talosctl service restart`.
  Automate later, only those three pass conditions.

## 7. Delivery order (reordered: prove blockers first)

1. **Vertical spike (release-blocking questions first):** minimal static
   binary + the final service YAML above; build with the chosen extension
   builder; install into a disposable Talos VM on sandbox01; prove (a) the
   machined container can mount iso9660 + tmpfs with the declared propagation,
   (b) the artifact passes `extensions-validator` and the publisher dry-run,
   (c) the nonce round-trip. Adjust the mount contract here if reality
   disagrees — not the core design.
2. **Production cutover:** organize the proven path into `internal/agent` +
   `internal/linux`, generate the four mocks, add the T1/T2 matrix, delete
   template placeholder code. (A1–A4/D1–D4 apply to the final tree, not the
   spike.)
3. **Release integration:** two-platform build, validator, existing
   publish/sign/SBOM chain; remove melange/apko once the gate passes.
4. **Docs + rehearsal:** raw-OCI install how-to, e2e runbook, one full live
   release rehearsal before v1.

### Deferred (explicitly out of v1)

- virtiofs media source (spike it only if removing the per-VM
  `agent:config` device-add step becomes a real operator need)
- Image Factory schematics; readiness/health endpoints; metrics; runtime
  configuration; `lxd-agent` legacy alias; media-update watching; internal
  child restart; Talos-host-root access; compatibility claims < v1.13.9

## 8. Deployment requirement (docs must state)

Every Talos VM needs the device added, e.g.:

```sh
incus config device add <vm> agent disk source=agent:config
```

Hot-add after boot is supported — the wrapper's retry loop picks it up.
