# Technical Notes

- Use hexagonal architecture at all times. Keep business logic isolated from CLI, filesystem, network, storage, and other external adapters.
- Prefer functional testing before calling any feature complete. Unit tests are useful, but they do not prove the tool works the way the design intends.
- Take an agile approach to development. Avoid waterfall: underspecify when useful, prototype early, learn from the result, and refine from working behavior.
- Mission: Talos system extension running the Incus guest agent so
  incus-spire-attestor can attest via /dev/incus/sock. Design authority:
  .journal/001/ARCHITECTURE.md; plan: .journal/001/PLAN.md (start Phase 1).
- Verified on Talos v1.13.9 / Incus 7.3: 9p absent from Talos kernel; the
  `agent:config` cdrom (iso9660, builtin) ships a host-matched static
  incus-agent + TLS material; no /dev/disk/by-label symlink (probe /dev/sr*);
  virtio_vsock autoloads. Stage to tmpfs /run/incus_agent, exec from there.
- Every Talos VM needs: incus config device add <vm> agent disk source=agent:config
- Live test host: sandbox01 (Ubuntu, Zabbly Incus; see ~/code/lab2/sandbox).
  Talos PodSecurity: privileged spike pods go in kube-system.
- Reference clones live in gitignored ref/ (siderolabs/extensions, lxc/incus);
  re-clone if missing.
