---
title: Why the static pod is privileged
---

# Why the static pod is privileged

The checked-in pod profile is privileged because narrower profiles failed the complete Incus behavior test. Privilege is a runtime requirement established by live testing, not a convenience setting.

## Required kernel operations

The wrapper must perform operations that ordinary containers cannot perform:

- discover optical block devices under `/dev/sr*`;
- mount the Incus configuration medium as read-only `iso9660`;
- mount a private tmpfs for the staged agent files;
- execute the host-supplied `incus-agent` as root;
- expose the host's AF_VSOCK device behavior to that agent; and
- preserve `/dev/incus/sock` access for node-local consumers.

The host-supplied agent reports guest information to Incus and serves the guest configuration socket. Both paths must work before a security profile is accepted.

## Live-tested reductions

The production acceptance test began with the full profile and changed one dimension at a time.

| Reduction | Observed result | Decision |
| --- | --- | --- |
| Remove `hostNetwork` | The wrapper staged and started, but Incus host guest reporting did not remain available. | Keep `hostNetwork: true`. |
| Replace host `/dev` with selected device paths | Media staging completed, but the real agent did not remain available to Incus and the socket consumer could not connect. | Keep the full host `/dev` mount. |
| Replace `privileged: true` with explicit capabilities and an unconfined seccomp request | The wrapper repeatedly failed to mount `/dev/sr0` with `operation not permitted`. | Keep `privileged: true`. |

No tested reduction passed the required host reporting and `/dev/incus/sock` nonce checks. The production profile therefore remains:

```yaml
hostNetwork: true
securityContext:
  privileged: true
  runAsUser: 0
  runAsGroup: 0
volumeMounts:
  - name: dev
    mountPath: /dev
```

The canonical manifest is [`deploy/talos/incus-guest-agent.yaml.tmpl`](https://github.com/componere/incus-guest-agent/blob/master/deploy/talos/incus-guest-agent.yaml.tmpl).

## Risk boundary

This pod can access the host device tree and perform privileged mount operations. Treat control of its image digest and Talos machine configuration as node-administrator access.

The deployment limits exposure in these ways:

- The machine configuration pins an immutable OCI digest.
- The image contains the wrapper and required Wolfi runtime files, but no shell or package manager.
- The image supports only Linux `amd64` and `arm64`.
- The wrapper copies only five named files from read-only ISO media.
- Staged files live in a private 50 MiB tmpfs with `nosuid` and `nodev`.
- The wrapper runs one host-supplied agent, supervises its whole process group, and reaps descendants.
- The agent does not require the Kubernetes API for normal operation or Talos-native diagnosis.

These controls do not make the pod unprivileged. Restrict who can publish the image, change the pinned digest, modify `machine.pods`, or attach Incus devices. Review any future privilege reduction against the same live matrix: cold boot, media attachment, host reporting, socket nonce, forced recovery, image update, Kubernetes API outage, consumer startup race, and reboot.
