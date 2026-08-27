---
title: Install on Talos under Incus
---

# Install on Talos under Incus

Install one digest-pinned static pod in each Talos virtual machine that needs the Incus guest agent.

## Prerequisites

You need:

- an Incus virtual machine running Talos Linux;
- Incus host access that can change the instance configuration;
- `talosctl` access to the target Talos node;
- Docker Buildx to resolve the release image digest;
- a local checkout of this repository for the canonical patch template; and
- public, anonymous read access to `ghcr.io/componere/incus-guest-agent`.

The canonical patch has no `imagePullSecrets`. A private GHCR package is not
supported by the proven profile. Before changing Talos, run the digest command
below from a workstation that has no GHCR credentials configured. Success
proves that the node can use the same anonymous registry path.


The production acceptance test used Incus 7.3 and Talos 1.13.9. Other versions have not passed this repository's live matrix.

The supported deployment is the `machine.pods` static pod in [`deploy/talos/incus-guest-agent.yaml.tmpl`](https://github.com/componere/incus-guest-agent/blob/master/deploy/talos/incus-guest-agent.yaml.tmpl).


## Attach the Incus configuration media

Run this command on the Incus host:

```sh
INSTANCE=talos-node-1
incus config device add "$INSTANCE" agent disk source=agent:config
```

If the `agent` device already exists, leave it attached. Inspect the current devices with:

```sh
incus config device show "$INSTANCE"
```

The wrapper can start before the media appears. It checks `/dev/sr*` every two seconds and starts the host-supplied agent after valid media is available.

## Resolve an immutable image digest

Choose a stable release number without the leading `v`, resolve its OCI index digest, and render the canonical patch:

```sh
IMAGE=ghcr.io/componere/incus-guest-agent
VERSION=1.2.3
DIGEST="$(docker buildx imagetools inspect \
  "$IMAGE:$VERSION" --format '{{.Manifest.Digest}}')"

case "$DIGEST" in
  sha256:*) ;;
  *) echo "unexpected image digest: $DIGEST" >&2; exit 1 ;;
esac

sed "s|sha256:<digest>|$DIGEST|" \
  deploy/talos/incus-guest-agent.yaml.tmpl \
  > incus-guest-agent.yaml
```

Keep the rendered patch and digest in your deployment records. The mutable release tag is only a lookup input; the Talos configuration must contain the resolved `sha256:` digest.

## Apply the patch to one node

Set the Talos API address and apply the rendered patch without rebooting:

```sh
NODE=10.0.0.10
TALOSCONFIG=./talosconfig

talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  patch machineconfig --mode no-reboot \
  --patch @incus-guest-agent.yaml
```

Repeat this operation one node at a time. Verify each node before changing the next node.

## Verify the wrapper and host-supplied agent

List the Kubernetes containers through the Talos API:

```sh
talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  containers --kubernetes
```

The output must contain a running `kube-system/incus-guest-agent-<node>:agent` container.

Read the wrapper log without using the Kubernetes API. Set `NODE_NAME` to the Talos node name:

```sh
NODE_NAME=talos-node-1

talosctl --talosconfig "$TALOSCONFIG" --nodes "$NODE" \
  logs --kubernetes --tail 50 \
  "kube-system/incus-guest-agent-${NODE_NAME}:agent"
```

A successful start contains these messages in order:

```text
waiting for Incus configuration media
staged Incus agent files device=/dev/sr0
starting host-supplied incus-agent
```

On the Incus host, confirm that the host can query and execute through the guest agent:

```sh
incus info "$INSTANCE"
incus exec "$INSTANCE" -- /usr/bin/incus-guest-agent --version
```

`incus info` must include live guest OS and address data. The version command must print the installed wrapper version.

## Verify a consumer of `/dev/incus/sock`

Set a fresh value on the Incus host:

```sh
NONCE="incus-agent-check-$(date +%s)"
incus config set "$INSTANCE" user.incus-agent-check "$NONCE"
```

From the static-pod workload that consumes Incus configuration, query the mounted socket:

```sh
curl --fail --silent --show-error \
  --unix-socket /dev/incus/sock \
  http://incus/1.0/config/user.incus-agent-check
```

The response must equal `$NONCE`. The consumer must mount the host's `/dev/incus/sock`; the production acceptance test used a privileged static pod with the host `/dev` mounted at `/dev`.

Remove the test value after verification:

```sh
incus config unset "$INSTANCE" user.incus-agent-check
```

For the required pod security profile and failure interpretation, see [Runtime and operations](../reference/runtime.md) and [Why the static pod is privileged](../explanation/privileged-static-pod.md).
