---
title: Runtime and operations reference
---

# Runtime and operations reference

## Deployment contract

The supported deployment is one Talos `machine.pods` static pod per Incus virtual machine.

| Field | Required value |
| --- | --- |
| Pod name | `incus-guest-agent` |
| Namespace | `kube-system` |
| Restart policy | `Always` |
| Image | `ghcr.io/componere/incus-guest-agent@sha256:<digest>` |
| Pull policy | `IfNotPresent` |
| Pod network | `hostNetwork: true` |
| Container user and group | `0:0` |
| Privilege | `privileged: true` |
| Device mount | host `/dev` mounted at container `/dev` |
| Platforms | `linux/amd64`, `linux/arm64` |

The OCI image metadata uses UID/GID 65532, but the static pod deliberately overrides it with UID/GID 0. See [Why the static pod is privileged](../explanation/privileged-static-pod.md).

A mutable tag is not a deployment identifier. Resolve a stable release tag once, then store the resulting OCI index digest in the machine configuration.

## Startup sequence

The wrapper performs this sequence:

1. Remove stale media and staging mounts under `/var/run/incus-guest-agent`.
2. Enumerate block devices that match `/dev/sr*` in lexical order.
3. Mount each candidate as read-only `iso9660` media.
4. Validate the five required regular files.
5. Mount a private tmpfs and stream-copy the files into it.
6. Unmount the optical medium.
7. Start the staged `incus-agent` in its own process group.
8. Reap the direct child and all reparented descendants.

Missing media, invalid media, discovery failures, and staging failures are retried every two seconds. The wrapper starts exactly one host-supplied agent after the first successful stage.

## Paths and files

| Item | Value |
| --- | --- |
| Optical-device glob | `/dev/sr*` |
| Runtime root | `/var/run/incus-guest-agent` |
| Media mountpoint | `/var/run/incus-guest-agent/media` |
| Staging tmpfs | `/var/run/incus-guest-agent/agent` |
| Staged executable | `/var/run/incus-guest-agent/agent/incus-agent` |
| tmpfs options | `mode=0700,size=50M,nosuid,nodev` |
| Copy temporary suffix | `.tmp` |

The media must contain these non-empty regular files:

- `incus-agent`, with at least one executable permission bit;
- `agent.conf`;
- `agent.crt`;
- `agent.key`; and
- `server.crt`.

The wrapper preserves permission bits. It copies each file to an exclusive temporary path, syncs and closes it, then renames it into place. Failed copies do not publish a partial final file.

The media mount is removed after staging. The staging tmpfs remains mounted for the lifetime of the host-supplied process and is removed when that process ends.

## Process shutdown

The wrapper is a child subreaper even when it is not Linux PID 1. It runs the host-supplied agent in a separate process group.

On `SIGINT` or `SIGTERM`, it:

1. sends `SIGTERM` to the process group;
2. waits 10 seconds;
3. sends `SIGKILL` if descendants remain; and
4. waits another 2 seconds for the process tree to disappear.

An unexpected agent exit makes the wrapper exit nonzero. `restartPolicy: Always` then lets kubelet replace the container, restage the media, and start a fresh agent.

## Command-line contract

```text
Usage: incus-guest-agent [--help] [--version]
```

- `--help` and `-h` print usage and exit 0.
- `--version` and `-v` print `incus-guest-agent <version>` and exit 0.
- Any other argument writes an error to standard error and exits 2.
- No arguments start the supervisor.

## Talos-native status and logs

These commands use the Talos API, not the Kubernetes API:

```sh
talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  containers --kubernetes

talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  logs --kubernetes --tail 50 \
  "kube-system/incus-guest-agent-${NODE_NAME}:agent"
```

A normal container row ends with `CONTAINER_RUNNING`. The image column can show the selected platform manifest digest rather than the multi-platform OCI index digest stored in the machine configuration.

### Wrapper log messages

| Message | Meaning | Operator action |
| --- | --- | --- |
| `waiting for Incus configuration media` | The supervisor started and is probing. | Attach or inspect the Incus `agent:config` device if the message persists. |
| `staged Incus agent files` | A valid medium was copied into tmpfs. | No action. The `device` field identifies the accepted optical device. |
| `starting host-supplied incus-agent` | The staged executable is starting. | Verify `incus info`, `incus exec`, and the consumer socket path. |
| `skipping invalid media` | A candidate under `/dev/sr*` lacked the required contract. | Inspect attached optical media; another candidate can still succeed. |
| `failed to discover optical devices` | Device enumeration failed. | Inspect the pod's `/dev` mount and Talos device state. |
| `failed to stage Incus media` | Mounting, validation, tmpfs setup, or copying failed. | Inspect the structured `device` and `err` fields. |
| `failed to clean runtime state` | A stale mount or runtime directory could not be removed. | Inspect mount state before forcing another restart. |

`mount /dev/sr0: operation not permitted` indicates that the container security profile cannot perform the required ISO mount. Use the checked-in privileged profile.

Warnings from the host-supplied agent about a connection closing can occur when a client such as `incus exec` disconnects. Confirm current host reporting and socket behavior before treating an isolated close warning as an outage.

## Behavior during Kubernetes API outages

The static pod and the host-supplied agent do not depend on kube-apiserver after kubelet has started them. During the production test, `kubectl` timed out while:

- the wrapper container remained running;
- an existing `/dev/incus/sock` consumer continued to receive fresh values; and
- `talosctl containers --kubernetes` and `talosctl logs --kubernetes` remained available.

Use the Talos-native commands above when the Kubernetes API is unavailable. Do not wait for a mirror Pod object before diagnosing the local static pod.

## Media removal and restart

After successful staging, removing the Incus `agent:config` device does not stop the current host-supplied process. A wrapper restart or node reboot creates a new tmpfs, so the wrapper waits until valid media is attached again.

Keep the media device attached in normal operation. Hot attachment is a recovery path, not the steady-state configuration.
