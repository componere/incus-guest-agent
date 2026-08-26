# Architecture: `incus-guest-agent` Talos static pod

Status: approved direction for implementation. This document supersedes
`.journal/001/ARCHITECTURE.md`. The extension-service design is retained only as
rejected-path history.

Evidence: session 002 live tests on Talos v1.13.9 and Incus 7.3, recorded in
`NOTES.md`, with spike source at commit `33f93b2` on branch
`spike/static-pod`.

## 1. Product boundary

`incus-guest-agent` is a small Linux process supervisor distributed as a
multi-architecture OCI application image. Talos runs the image as a static pod
from `machine.pods` on every Incus-backed node.

The wrapper discovers the Incus `agent:config` CD-ROM, stages the host-supplied
files into a private tmpfs, and runs the host-matched `incus-agent`. The v1
contract is one observable result: `/dev/incus/sock` works inside the Talos VM
so consumers such as `incus-spire-attestor` can read claims and challenge
nonces.

The product does not include a Kubernetes controller, DaemonSet, sidecar,
socket proxy, Talos system extension, or bundled copy of `incus-agent`.

## 2. Decisions fixed by evidence

- **Static pod, not extension service.** Talos extension services always receive
  containerd's default seccomp profile. That profile excludes `AF_VSOCK`, and
  the extension-service schema has no override. The real agent failed with
  `operation not permitted` on that path. A privileged static pod ran the same
  host-supplied agent and opened the vsock listener.
- **`machine.pods` owns deployment.** Talos serves static pod configuration to
  kubelet without depending on the Kubernetes API server. Kubelet owns
  container creation and restart. Machine-config updates own rollout and
  rollback on each node.
- **CD-ROM is the only v1 media source.** Talos v1.13.9 has no 9p support.
  Virtiofs remains unproven. Probe lexically sorted `/dev/sr*` devices because
  Talos creates no `/dev/disk/by-label/incus-agent` symlink.
- **Never bundle `incus-agent`.** Incus supplies a static binary matched to the
  host version plus `agent.conf`, `agent.crt`, `agent.key`, and `server.crt`.
  Bundling would introduce version skew and another release lifecycle.
- **Retain Melange and apko.** The product is now an application image, not an
  extension payload. The repository's existing Go → Melange apk → apko OCI
  pipeline matches that artifact and already supplies multi-architecture
  assembly, SBOMs, signing, attestations, and vulnerability scanning.
- **Keep the CLI surface minimal.** Normal execution runs the service. `--help`
  and `--version` are the only flags. There is no config file, environment
  configuration, API, health endpoint, readiness endpoint, metrics surface, or
  Cobra/Viper dependency.
- **Consumers own startup retry.** `/dev/incus/sock` can appear after consumers
  start. The live consumer-starts-first test recovered without either pod
  restarting. The agent pod does not impose Kubernetes startup ordering.

## 3. Runtime topology

```text
Incus host
  └─ agent:config device
       └─ Talos VM /dev/sr*
            └─ kubelet static pod: incus-guest-agent
                 ├─ wrapper PID 1
                 ├─ private iso9660 mount
                 ├─ private tmpfs staging area
                 └─ host-supplied incus-agent
                      └─ /dev/incus/sock on the VM's host-mounted /dev
                           └─ incus-spire-attestor and other consumers
```

Talos machine configuration contains the static pod definition. The pod mounts
the VM's `/dev` as a host path. The wrapper's media and tmpfs mounts remain in
the workload container's mount namespace. A forced container exit destroys
that namespace; no staging mount propagates back to the Talos host.

The Kubernetes API server is not in the runtime chain. During the live test,
pausing kube-apiserver made authenticated `kubectl` time out while the agent,
consumer, `talosctl containers --kubernetes`, and `talosctl logs --kubernetes`
continued to work.

## 4. Static pod contract

The canonical manifest is a Talos machine-config patch template under
`deploy/talos/`. It has one image placeholder. Operators must replace the
placeholder with the released GHCR reference pinned by digest before applying
the patch.

The verified baseline is:

```yaml
machine:
  pods:
    - apiVersion: v1
      kind: Pod
      metadata:
        name: incus-guest-agent
        namespace: kube-system
      spec:
        restartPolicy: Always
        hostNetwork: true
        containers:
          - name: agent
            image: ghcr.io/componere/incus-guest-agent@sha256:<digest>
            imagePullPolicy: IfNotPresent
            securityContext:
              privileged: true
              runAsUser: 0
              runAsGroup: 0
            volumeMounts:
              - name: dev
                mountPath: /dev
        volumes:
          - name: dev
            hostPath:
              path: /dev
              type: Directory
```

`kube-system` gives mirror-pod visibility when the API server is available, but
mirror pods are not the control plane for this workload. Talos-native status and
logs are authoritative.

The baseline is intentionally explicit about its privilege:

- `privileged: true` bypasses the seccomp and device-cgroup restrictions that
  blocked AF_VSOCK and permits mounting the CD-ROM and tmpfs.
- UID/GID 0 matches the only live-proven execution profile.
- `hostNetwork: true` preserves the VM network view expected from a guest agent.
- Host `/dev` exposes optical media, vsock, and the shared socket path.
- The pod does not request host PID or host IPC namespaces.

Implementation may reduce this profile only after a live test proves the same
cold-boot, hot-add, nonce, restart, and host-reporting behavior. Unproven
privilege reduction must not replace the verified baseline.

Every VM also requires the Incus device:

```sh
incus config device add <vm> agent disk source=agent:config
```

Hot-add is supported. The wrapper continues probing until valid media appears.

## 5. Wrapper behavior

Fixed v1 values:

| Item | Value |
|---|---|
| Probe | Lexically sorted block devices matching `/dev/sr*` |
| Poll interval | 2 seconds while no valid medium exists |
| Runtime root | `/var/run/incus-guest-agent` |
| Media mount | `media`, read-only `iso9660` |
| Stage mount | `agent`, tmpfs `mode=0700,size=50M`, `nosuid,nodev` |
| Required files | `incus-agent`, `agent.conf`, `agent.crt`, `agent.key`, `server.crt` |
| Exec | staged `incus-agent`, staged directory as CWD, no arguments or shell |
| Shutdown | SIGTERM to the process group; SIGKILL after 10 seconds; fail after 2 more seconds |

For each candidate, the wrapper mounts the device read-only and requires all
five entries to be nonempty regular files. `incus-agent` must have an executable
mode bit. The wrapper creates the staging tmpfs, streams each file without
buffering the payload, preserves permission bits, unmounts the CD-ROM, and
starts the staged agent.

Missing or invalid media is retryable. Preparation errors are logged and
retried; they do not permanently disable the pod. The first valid medium wins.
The v1 process does not watch for media replacement after the agent starts.

The wrapper is container PID 1 and also enables Linux child-subreaper behavior.
It starts the agent in a separate process group, forwards shutdown signals,
waits for every direct or reparented descendant, and reports unexpected agent
exit as failure. It does not restart the child internally. A nonzero wrapper
exit lets kubelet's `restartPolicy: Always` recreate the whole container and
reconcile from a clean mount namespace.

## 6. Code boundaries

```text
cmd/incus-guest-agent/
  doc.go                 composition root and version/help contract
  main.go
internal/agent/
  doc.go                 side-effect-free orchestration and domain terms
  agent.go
  ports.go
  mocks/                 mockery-generated port mocks
internal/linux/
  doc.go                 Linux implementations of the four ports
  device.go              optical-device discovery
  stage.go               mount, validate, and stream-copy transaction
  process.go             process group, signals, and descendant reaping
  wait.go                cancelable poll delay
deploy/talos/
  incus-guest-agent.yaml.tmpl
```

`internal/agent` owns four interfaces:

| Port | Contract |
|---|---|
| `DeviceFinder` | Return sorted `DevicePath` candidates. No mounting. |
| `StageManager` | Mount and validate one candidate, stage the five files transactionally, and clean its mounts. |
| `AgentProcess` | Run the staged agent process tree until exit or requested shutdown. |
| `Waiter` | Wait for the fixed poll interval or cancellation. |

Interfaces remain in the consumer package. Production constructors accept the
interfaces and return concrete values. `DevicePath` is a domain type because it
crosses the core boundary. All four mocks are generated with Mockery; none are
written by hand.

The core decides when to probe, retry, start, and return. All filesystem,
mount, process, signal, and clock effects stay in Linux adapters. Every package,
function, type, and field receives Godoc as required by repository rules.

The template packages and `cmd/template-go` are deleted in the implementation
cutover. There are no compatibility aliases or deprecated entry points.

## 7. Image and release architecture

GoReleaser produces the canonical static Linux amd64 and arm64 binaries with
version, commit, and build date. Darwin, Windows, native packages, Homebrew, and
Scoop are outside this product.

Melange packages those exact binaries into one signed apk per architecture.
apko composes the two-architecture Wolfi image from the apk and minimal runtime
packages. The image contains no shell or package manager. Its default account
remains nonroot UID/GID 65532; the canonical static pod deliberately overrides
the account to the verified root profile.

The existing pinned `meigma/release` reusable workflows remain the release
boundary:

1. Go pre-publish creates and verifies canonical Linux artifacts.
2. OCI build verifies the artifact handoff, builds the Melange packages and apko
   layout, checks both platform manifests and embedded bytes, and emits SBOMs.
3. OCI publish verifies the layout, publishes immutable GHCR tags, signs the
   image, and attaches provenance and per-platform SBOM attestations.
4. GitHub release publication consumes the verified handoffs.

Release Please remains the sole version source. The weekly Trivy workflow
rebuilds the image from the same `melange.yaml` and `apko.yaml` before scanning
HIGH and CRITICAL OS and library findings.

## 8. Deployment lifecycle and observability

Installation, update, and rollback are per-node machine-config operations, not
Kubernetes rollouts.

- **Install:** add the Incus `agent:config` device, render the static pod template
  with an immutable image digest, and apply it to every target node.
- **Update:** replace the digest in each node's machine configuration. The live
  spike proved that Talos updates the static pod without a reboot or manual
  kubelet restart.
- **Rollback:** reapply the previous known-good digest to each affected node.
- **Cold boot:** kubelet starts the cached image or pulls it from GHCR. The
  wrapper waits if media is absent.
- **Runtime failure:** kubelet replaces the failed wrapper container. The new
  mount namespace stages media again.

Primary operator commands:

- `talosctl get staticpods`
- `talosctl get staticpodstatus`
- `talosctl containers --kubernetes`
- `talosctl logs --kubernetes <container-id>`

The wrapper logs state transitions and actionable failures to stderr. It does
not create another health protocol. Socket consumers must retry connection
while `/dev/incus/sock` is absent or has no listener.

## 9. Verification strategy

- **T1 core tests:** no media retries until cancellation; candidates remain
  sorted; invalid media does not start a process; preparation failures retry;
  successful staging starts exactly one process; requested shutdown and
  unexpected process exit produce different results.
- **T2 adapter tests:** validate and stream-copy the five-file contract in temp
  directories; exercise process-group signal forwarding and descendant reaping
  with helper processes. Generate mocks with Mockery. Do not fake privileged
  mounts and claim they prove Linux behavior.
- **Image checks:** local apko image build and `--version` smoke; canonical binary
  handoff; exact linux/amd64 and linux/arm64 index; embedded binary verification;
  SBOM, signature, provenance, and Trivy gates.
- **T3 live Talos test:** cold boot before media, hot-add, real nonce through a
  second pod, forced wrapper kill and recovery, digest update without reboot,
  consumer-starts-first recovery, and Talos-native inspection while the API
  server is unavailable.

The static-pod spike already proves T3 for the prototype. Production bytes and
the final manifest must pass the same matrix before v1 publication.

## 10. Deferred and rejected scope

Deferred beyond v1:

- virtiofs or another media source
- automatic node rollout or a Kubernetes operator
- Image Factory integration
- readiness, liveness, metrics, or runtime configuration
- media replacement while the agent is running
- compatibility claims earlier than Talos v1.13.9
- least-privilege changes not backed by the live matrix

Rejected for v1:

- Talos extension service: AF_VSOCK is blocked by its mandatory seccomp profile.
- DaemonSet: depends on API scheduling and admission for a node-critical service.
- Bundled `incus-agent`: creates host/guest version skew.
- Socket proxy or sidecar: adds another protocol and failure boundary.
- Custom OCI assembler or Sidero extension builder: unnecessary for an
  application image; Melange/apko already matches the release contract.
- Internal child restart: duplicates kubelet and can preserve bad runtime state.

## 11. Completion boundary

The v1 architecture is complete when production code, the canonical static pod
template, the Melange/apko image, release workflows, operator documentation,
and the production-image live matrix all implement this document with one
runtime path and no remaining template identity.