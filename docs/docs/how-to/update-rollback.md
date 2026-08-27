---
title: Update and roll back
---

# Update and roll back

Change the image digest one Talos node at a time. A no-reboot machine-config patch replaces only the static pod whose image changed.

## Record the current deployment

Before an update, record:

- the current `sha256:` OCI index digest;
- the rendered Talos patch that contains that digest;
- the wrapper version reported through `incus exec`; and
- a successful `/dev/incus/sock` nonce result.

The previous rendered patch is the rollback input. Do not rely on a mutable image tag to reconstruct it.

## Resolve and render the new digest

Resolve the stable release tag to an immutable digest:

```sh
IMAGE=ghcr.io/componere/incus-guest-agent
VERSION=1.2.4
NEW_DIGEST="$(docker buildx imagetools inspect \
  "$IMAGE:$VERSION" --format '{{.Manifest.Digest}}')"

case "$NEW_DIGEST" in
  sha256:*) ;;
  *) echo "unexpected image digest: $NEW_DIGEST" >&2; exit 1 ;;
esac

sed "s|sha256:<digest>|$NEW_DIGEST|" \
  deploy/talos/incus-guest-agent.yaml.tmpl \
  > incus-guest-agent-1.2.4.yaml
```

Keep both the old and new rendered patches until the rollout is complete.

## Update one node

```sh
NODE=10.0.0.10
NODE_NAME=talos-node-1
INSTANCE=talos-node-1
TALOSCONFIG=./talosconfig

talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  patch machineconfig --mode no-reboot \
  --patch @incus-guest-agent-1.2.4.yaml
```

Watch the Talos-native container list until a new agent container is running:

```sh
talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  containers --kubernetes
```

Then check the wrapper state transitions and version:

```sh
talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  logs --kubernetes --tail 50 \
  "kube-system/incus-guest-agent-${NODE_NAME}:agent"

incus exec "$INSTANCE" -- /usr/bin/incus-guest-agent --version
incus info "$INSTANCE"
```

Complete a fresh `/dev/incus/sock` nonce check as described in [Install on Talos under Incus](install.md#verify-a-consumer-of-devincussock).

If all checks pass, update the next node. Talos does not coordinate `machine.pods` changes as a Kubernetes rollout; the operator owns node ordering and failure containment.

## Roll back one node

Apply the previous rendered patch with the same no-reboot operation:

```sh
talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  patch machineconfig --mode no-reboot \
  --patch @incus-guest-agent-previous.yaml
```

Confirm that:

1. `containers --kubernetes` shows a new running agent container;
2. the log shows media staging and the host-supplied agent start;
3. `incus exec ... --version` reports the previous wrapper version;
4. `incus info` reports live guest data; and
5. a fresh socket nonce succeeds.

Stop the rollout and roll back every changed node if the failure is common to the new release. Preserve the failing digest and logs for diagnosis.

## Reboot behavior

A no-reboot patch persists in the Talos machine configuration. A later reboot starts the static pod from the configured digest.

`imagePullPolicy: IfNotPresent` lets kubelet reuse a cached image that matches the digest. If the image is not cached, the node needs registry access during startup.

The wrapper copies the five media files into tmpfs before starting the host-supplied agent. Removing `agent:config` after a successful start does not stop that running process, but tmpfs is lost when the pod or node restarts. Keep `agent:config` attached so restart and reboot recovery do not wait for media.
