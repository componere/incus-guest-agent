---
title: incus-guest-agent
slug: /
description: Operate the Incus guest agent in Talos virtual machines.
---

# incus-guest-agent

`incus-guest-agent` stages and supervises the host-supplied Incus guest agent
inside a privileged Talos static pod. Deploy it only through the canonical
`machine.pods` manifest with an immutable OCI digest.

Use these documents to operate it:

- [Install on Talos under Incus](how-to/install.md)
- [Update and roll back](how-to/update-rollback.md)
- [Runtime and operations reference](reference/runtime.md)
- [Why the static pod is privileged](explanation/privileged-static-pod.md)

