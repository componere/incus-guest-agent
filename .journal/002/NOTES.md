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

## 2026-08-26 12:33 — Static pod alternative is source-valid but unproven
Reviewed the proposed `machine.pods` alternative against Talos v1.13.9, its pinned containerd fork, Kubernetes static-pod documentation, the current image configuration, and session 001 evidence.

Confirmed: Talos converts `machine.pods` entries into `StaticPod` resources, serves the complete pod list on loopback HTTP through kubelet's `staticPodURL`, accepts updates without reboot, and exposes runtime state through `staticpodstatus` and the Kubernetes container namespace. Static pods bypass API-server admission. Talos extension services always receive containerd's default seccomp profile unless an internal runner override is supplied; the extension-service schema exposes no override. That profile explicitly excludes `AF_VSOCK`. The CRI path returns no seccomp spec option for a privileged container before considering Talos's `RuntimeDefault` setting, so a privileged static-pod workload container should avoid the observed EPERM.

Corrections and gaps before approval:

1. Source analysis makes the design plausible, not proven. Build a runnable OCI image and repeat the real-agent nonce round trip as a static pod before revising the architecture.
2. The sample manifest inherits the image user. The current `apko.yaml` sets UID/GID 65532, while session 001 proved a root privileged pod. Set `runAsUser: 0` for an equivalent spike or explicitly prove the nonroot all-capabilities case.
3. PodSecurity does not gate execution, but a privileged mirror pod in the default namespace may be rejected by API admission. Use `kube-system` if mirror-pod observability is required, or document that Talos-native status/log commands are authoritative.
4. Calling the privilege delta cosmetic is inaccurate. The static-pod workload intentionally removes seccomp and `no_new_privileges`; the extension service retains those controls and a read-only rootfs by default, although it already receives all capabilities, all devices, and host network/IPC namespaces.
5. Hot updates are per-node Talos machine-config operations, not a Kubernetes rollout. Production instructions must cover applying and rolling back the config on every target node and use an immutable image digest.
6. A true scratch image and the current melange/apko Wolfi image are different packaging choices. Choose one. Reusing the current pipeline also requires changing its nonroot runtime account or overriding it in the pod.

Decision: approve a disposable static-pod spike, not the architecture revision yet. Acceptance is the same real-agent chain as session 001 plus cold boot, hot media add, forced container restart, machine-config image update, and no-API-server observability.

## 2026-08-26 12:59 — Static pod passes the live acceptance matrix
Built and published a disposable two-architecture Wolfi image from the proven wrapper, then cold-booted a fresh Talos v1.13.9 single-node cluster as Incus VM `talos-agent-staticpod-spike` on `sandbox01`. The pod ran in `kube-system` as UID/GID 0 with `privileged: true`, `hostNetwork: true`, and `/dev` mounted from the host. The evidence source is commit `33f93b2` on pushed branch `spike/static-pod`.

Results:

1. Cold boot and hot-add pass. The static pod started before agent media existed and logged `waiting for Incus configuration media under /dev/sr*`. Hot-adding `agent:config` exposed `/dev/sr0`; the wrapper staged the five required files and started the real host-supplied Incus agent.
2. The Talos seccomp blocker is absent on this path. The real agent opened its AF_VSOCK listener rather than failing with `operation not permitted`. Incus reported guest OS, kernel, process, disk, memory, and network data.
3. The real nonce round trip passes through a second pod. After setting `user.staticpod-nonce=staticpod-roundtrip-20260826`, a separate static pod mounted host `/dev` and repeatedly read that exact value through `/dev/incus/sock`.
4. Forced workload recovery passes. SIGKILL of the wrapper host PID replaced agent container `9248e917697c` with `9abbfe6f4ca9`; the replacement restaged `/dev/sr0`, restarted the host agent, and the consumer continued receiving the nonce.
5. Per-node image update passes without reboot or a manual kubelet restart. A `talosctl patch machineconfig --mode no-reboot` change from immutable image A (`sha256:87bfbd9f…`) to image B (`sha256:7ae9b740…`) updated the `StaticPod` resource, replaced the agent container with `e133ef0fe0ae`, and preserved nonce responses.
6. API-independent observability passes. SIGSTOP paused the kube-apiserver host process; authenticated `kubectl` failed with `context deadline exceeded`, while `talosctl containers --kubernetes` still listed both static pods and `talosctl logs --kubernetes` still returned agent and live nonce output. SIGCONT restored `/readyz` to `ok`.
7. The consumer-starts-first race passes. With agent media detached, a fresh consumer container `18b938c64df4` logged connection failures. Reattaching `agent:config` caused the already-running wrapper to stage media and start the agent; the same consumer transitioned to successful nonce responses on its next retry without restart.

Decision: the privileged static-pod execution model resolves the release-blocking AF_VSOCK issue and passes every authorized live acceptance condition. It should replace the extension service as the production architecture candidate. The spike does not establish least privilege: production work still must minimize the pod's privileges and host mounts without breaking AF_VSOCK or `/dev/incus/sock`, define per-node image rollout and rollback, and preserve Talos-native observability. Do not edit the stored architecture or implementation plan until the user authorizes that now-evidence-backed revision.

Teardown complete: deleted `talos-agent-staticpod-spike`, removed generated remote/local Talos assets, kubeconfig, signing keys, apk repositories, compiled binaries, and local OCI archives. The pushed `spike/static-pod` branch and 24-hour OCI references remain as durable and short-lived evidence respectively.
