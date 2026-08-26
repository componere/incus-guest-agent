# Implementation plan: ship the static-pod guest agent

Status: execution plan for `.journal/002/ARCHITECTURE.md`. This document
supersedes `.journal/001/PLAN.md`.

The implementation begins from current `master`; neither spike branch is a
merge source. Reuse proven behavior and tests selectively, but make production
changes as a clean replacement of the repository template.

## 1. Evidence and starting point

Two disposable spikes removed the release-blocking uncertainty:

- `spike/runtime` commit `491f43c`: the wrapper can discover the Incus CD-ROM,
  validate and stage all five files, supervise PID 1 descendants, shut down
  cleanly, and recover through Talos extension-service restarts. It also proved
  that Talos's mandatory extension-service seccomp profile blocks the real
  agent's AF_VSOCK listener.
- `spike/static-pod` commit `33f93b2`: a privileged static pod can run the real
  host-supplied agent. It passed cold boot, hot-add, nonce challenge, forced
  container recovery, digest update, API-outage observation, and the
  consumer-starts-first race on Talos v1.13.9 and Incus 7.3.

Current `master` is still the Meigma Go application template. It already has the
release supply-chain machinery to retain: GoReleaser, Melange, apko, Release
Please, pinned reusable release workflows, Trivy, mise, Moon, Mockery, and the
documentation site.

## 2. Delivery rules

- Work in one isolated implementation worktree created from a freshly fetched
  `origin/master`. Commit small coherent slices and push the branch regularly.
- Treat `.journal/002/ARCHITECTURE.md` as the current product contract. If live
  evidence disproves it, append the result to session notes and revise the
  artifact before continuing; do not add a hidden second path.
- Cut over cleanly. Remove the template CLI, packages, configuration, tests,
  docs, and release surfaces when their production replacement lands. Do not
  retain compatibility aliases.
- Use immutable image digests in Talos configuration. Mutable tags are release
  discovery aids, not deployment identity.
- Do not weaken release gates to make the new artifact pass. Correct the source,
  package, or workflow input.
- Do not claim least privilege without rerunning the full live matrix on the
  reduced profile.

## 3. Phase 1 — production runtime and local image

**Goal:** one production code path builds and runs as a Linux amd64/arm64 image,
with the canonical Talos manifest present in the repository.

### 3.1 Create the implementation branch

1. Fetch `origin/master`.
2. Create an isolated Worktrunk worktree named for the static-pod
   implementation.
3. Confirm the worktree is clean and based on the fetched master commit.
4. Use the branch only for production work; keep both spike branches as
   read-only evidence.

### 3.2 Replace template identity and dependency surface

Modify the module and composition root in one cutover:

- Rename the module and imports from the template identity to
  `github.com/componere/incus-guest-agent`.
- Replace `cmd/template-go` with `cmd/incus-guest-agent`.
- Implement normal service execution plus `--help` and `--version`; reject all
  other arguments on stderr with a nonzero status.
- Preserve linker-injected version, commit, and build-date variables needed by
  GoReleaser, but expose only the concise `--version` output to users.
- Delete Viper, Cobra, Charmbracelet, template-info, sample config, and every
  template-only package and test.
- Run `go mod tidy` after the cutover and confirm no deleted CLI dependency
  remains.

Deliverable: `go run ./cmd/incus-guest-agent --version` prints the production
name and exits zero; normal execution enters the service path.

### 3.3 Implement the core and Linux adapters

Create the architecture's two internal packages:

```text
internal/agent/
  doc.go
  agent.go
  ports.go
  mocks/
internal/linux/
  doc.go
  device.go
  stage.go
  process.go
  wait.go
```

`internal/agent`:

1. Define `DevicePath` and the `DeviceFinder`, `StageManager`, `AgentProcess`,
   and `Waiter` consumer-owned ports.
2. Implement the no-media and preparation-failure retry loop with a fixed
   two-second wait.
3. Stop retrying on context cancellation.
4. Start exactly one process after a complete successful stage.
5. Return different errors for unexpected agent exit and requested shutdown so
   the composition root can choose the container exit code.

`internal/linux`:

1. Enumerate only block devices matching `/dev/sr*` and return lexical order.
2. Create `/var/run/incus-guest-agent/media` and `agent`; mount candidate media
   read-only as iso9660.
3. Require nonempty regular files named `incus-agent`, `agent.conf`,
   `agent.crt`, `agent.key`, and `server.crt`; require one executable bit on the
   binary.
4. Mount tmpfs with `mode=0700,size=50M`, `nosuid`, and `nodev`.
5. Stream-copy each source into a temporary destination, preserve its permission
   bits, sync, close, and atomically rename only after success.
6. Unmount media before process launch. On every failed attempt, unmount any
   mount created by that attempt and remove incomplete staging output.
7. Enable Linux child-subreaper mode, launch the staged binary in its own
   process group and with the staging directory as CWD, and reap direct and
   orphaned descendants.
8. On shutdown, send SIGTERM to the group, wait ten seconds, send SIGKILL, wait
   two more seconds, then fail if descendants remain.
9. Forward agent stdout and stderr without buffering.

Use the static-pod spike to recover exact proven syscall ordering, but review
and reshape it around these ports instead of copying its single package whole.
Add Godoc to every package, type, field, and function.

### 3.4 Add behavior-focused tests

Generate all four port mocks with the repository's Mockery configuration.
Generated code lives in `internal/agent/mocks`.

Add T1 table tests for:

- no media → wait → probe again
- invalid candidate → next candidate
- all invalid candidates → wait and retry
- stage error → cleanup contract and retry
- cancellation while waiting → clean shutdown result
- successful stage → one process run
- unexpected process exit → failure result
- requested shutdown → clean result

Add T2 Linux adapter tests that provide observable value without pretending to
be privileged mount tests:

- candidate filtering and lexical order
- complete and incomplete five-file validation
- zero-length and non-regular file rejection
- executable-bit enforcement
- streamed copy preserves modes and leaves no partial final file on failure
- helper-process test for process-group SIGTERM forwarding
- helper-process test for orphan adoption and reaping
- SIGKILL escalation after the grace interval using an injected clock/wait port
  rather than a real ten-second test

Run the narrow package tests while developing, then run the repository's normal
Go test and lint tasks once the slice is integrated.

### 3.5 Add the canonical Talos patch template

Create `deploy/talos/incus-guest-agent.yaml.tmpl` with the exact manifest from
`ARCHITECTURE.md`:

- `machine.pods`
- pod `incus-guest-agent` in `kube-system`
- `restartPolicy: Always`
- `hostNetwork: true`
- one container named `agent`
- `imagePullPolicy: IfNotPresent`
- digest-pinned GHCR image placeholder
- explicit `privileged: true`, UID 0, and GID 0
- host `/dev` mounted at `/dev`
- no host PID or IPC namespace
- no probes, sidecars, init containers, service, RBAC, or API object

Add a small deterministic renderer/validator only if an existing repository tool
already needs one. Do not create a deployment framework around a one-placeholder
YAML template.

### 3.6 Retarget Melange and apko

Update `melange.yaml`:

- package name, description, copyright, and source URL
- canonical binary input name for amd64 and arm64
- installed path `/usr/bin/incus-guest-agent`
- package annotations and version expressions

Update `apko.yaml`:

- install the renamed apk
- entrypoint `/usr/bin/incus-guest-agent`
- product title, description, source, and licenses
- keep only the minimal Wolfi runtime packages needed by the final image
- retain amd64 and arm64
- retain the nonroot image account; the pod manifest owns the root override

Update `mise.toml`, Moon inputs, and local-image tasks so GoReleaser artifacts,
Melange staging, apko assembly, SBOM output, and smoke commands all use the
production names. Reuse pinned tools from `mise.lock`; do not introduce Dockerfile
or custom assembler paths.

### 3.7 Prove the local image

1. Build production linux/amd64 and linux/arm64 binaries with the same flags
   expected by GoReleaser.
2. Build both Melange packages using the pinned tool.
3. Build the multi-architecture apko layout or archive.
4. Verify the index contains exactly linux/amd64 and linux/arm64.
5. Extract or run each image's entrypoint and assert the expected `--version`
   output.
6. Inspect the image filesystem to confirm the renamed binary is present, the
   template binary is absent, and no shell or package manager was added.
7. Retain the per-build SBOM as evidence; remove local keys, archives, and
   staging directories after the checks.

### Phase 1 acceptance

- Production code has one runtime path and no template packages.
- T1 and T2 tests pass.
- `go vet` and the configured linter pass.
- The canonical static pod patch matches the approved privileged baseline.
- A local OCI artifact contains exactly two architectures and runs the
  production `--version` contract on both.
- The worktree is clean after committing and pushing the phase.

## 4. Phase 2 — supply-chain and release cutover

**Goal:** every existing CI and release gate consumes the renamed product and
its canonical static-pod application image.

### 4.1 Trim GoReleaser to the product

Update `.goreleaser.yaml`:

- project and binary name `incus-guest-agent`
- main package `./cmd/incus-guest-agent`
- Linux only, amd64 and arm64 only
- `CGO_ENABLED=0`, trimpath, reproducible build flags, and version/commit/date
  linker variables
- retain archives, checksums, source archive, SBOM generation, checksum
  Sigstore bundle, `gomod.proxy`, and the release-disabled handoff used by the
  reusable workflows
- remove Darwin, Windows, native packages, Homebrew, Scoop, macOS signing, and
  other template distribution surfaces

Run a local GoReleaser snapshot. Assert the exact two binaries expected by the
Melange `pipeline.sources` contract and inspect their embedded version metadata.

### 4.2 Update repository metadata and automation inputs

Replace remaining template identity in:

- `release-please-config.json` and `.release-please-manifest.json`
- root and project Moon tasks
- code, docs, SBOM, and container task input sets
- `.github/workflows/ci.yml`
- `.github/workflows/release.yml`
- `.github/workflows/security-scan.yml`
- dependency and path filters that reference the old command or package names

Keep the existing pinned action and reusable-workflow SHAs, minimal permissions,
OIDC flow, artifact-digest checks, signing, provenance, and vulnerability gates.
Do not reimplement release logic in this repository.

The release workflow remains:

```text
Go pre-publish
  → OCI build
  → OCI publish
  → GitHub release publish
```

The weekly security workflow must build from the same production
`melange.yaml` and `apko.yaml`, scan HIGH/CRITICAL OS and library findings, and
upload SARIF with no template image reference.

### 4.3 Rehearse without publishing

Use an unused stable-format disposable tag on the implementation commit because
the reusable release chain validates semantic release tags.

For the rehearsal commit only:

- set OCI publication false
- set GitHub release publication false
- set the downstream publisher not to require an OCI reference
- leave signing, package assembly, index validation, SBOM generation, embedded
  binary checks, and artifact-digest verification enabled

Push the disposable tag, observe every build and verification job, and inspect
artifacts for both platforms. Delete the remote and local tag after the run.
Revert the rehearsal-only switches to the production settings before merge.

If a reusable-workflow contract fails, fix the caller input or product artifact.
Do not fork the workflow or skip its validator.

### Phase 2 acceptance

- Local GoReleaser snapshot has only linux/amd64 and linux/arm64.
- CI, security scan, and release workflows contain no template command, package,
  or image identity.
- The no-publish rehearsal passes canonical-artifact, Melange, apko, platform,
  embedded-binary, SBOM, and provenance preparation gates.
- Production publication switches are restored.
- Release Please remains the only version source.
- The worktree is clean after committing and pushing the phase.

## 5. Phase 3 — production deployment proof and operator docs

**Goal:** production-built bytes pass the proven live matrix, and operators have
one exact install, update, rollback, and troubleshooting path.

### 5.1 Test privilege reductions one dimension at a time

Start with the approved manifest unchanged. Only after the production image
passes the baseline may the implementation attempt reductions. Test each change
alone and restore the last known-good manifest after a failure:

1. remove `hostNetwork`
2. replace full host `/dev` with the smallest device/path set that can expose
   optical media, AF_VSOCK, and `/dev/incus/sock`
3. replace `privileged: true` with explicit capabilities, device access, and a
   seccomp profile

A candidate reduction is accepted only if all live checks in 5.2 still pass,
including real Incus host guest-info and nonce behavior. If no complete reduced
profile passes, ship the explicit privileged baseline and document the reason.

### 5.2 Repeat the live matrix with production bytes

Provision a disposable Incus VM with the supported Talos version and the exact
production-image digest. Capture commands and results for:

1. **Cold boot before media:** pod starts first, waits, then stages and launches
   after `agent:config` is attached.
2. **Real agent readiness:** Incus host reports guest OS and address data.
3. **Nonce round trip:** a second pod reaches `/dev/incus/sock` and completes a
   fresh challenge.
4. **Forced recovery:** kill the wrapper/container, observe kubelet replacement,
   new wrapper PID, restaging, and another successful nonce.
5. **Image update:** patch the machine config from digest A to digest B in
   no-reboot mode; observe only the intended static pod update and verify a new
   nonce.
6. **API outage:** pause kube-apiserver; show `kubectl` failure while agent and
   consumer continue and Talos-native container/status/log commands work.
7. **Consumer startup race:** start a fresh consumer while agent media is absent,
   then attach media and observe recovery without restarting either pod.
8. **Reboot:** cold reboot the node with the image and device configured; verify
   the socket, host reporting, and nonce after startup.

Record Talos, Incus, image digest, machine-config patch, wrapper version, and the
accepted security profile. Tear down the VM, generated credentials, temporary
keys, images, and local/remote staging artifacts after evidence is captured.

### 5.3 Replace template documentation

Use the repository's existing `docs/` Docusaurus/Moon structure. Delete the
sample template pages and create only user-facing documentation required by the
product:

- **How-to: install on Talos under Incus** — prerequisites, attach
  `agent:config`, obtain the release digest, render the canonical patch, apply
  it to every node, and verify with Talos-native commands and a nonce-capable
  consumer.
- **How-to: update and roll back** — per-node digest replacement, staged rollout,
  success checks, rollback to a known digest, and reboot implications.
- **Reference: runtime and operations** — fixed paths, required media files,
  retry interval, signal deadlines, pod security requirements, image platforms,
  log messages, static-pod status commands, and API-outage behavior.
- **Explanation: why a static pod is privileged** — the AF_VSOCK seccomp
  constraint, mount/device needs, risk boundary, and any live-proven reductions.

Update the repository README only where it routes users into these docs. Remove
all template application examples, config instructions, screenshots, and
unsupported installation methods. Do not document a tag-based deployment path
or Kubernetes API-managed pod.

### Phase 3 acceptance

- Production-image digest passes every live check in 5.2.
- The checked-in manifest exactly matches the successful live security profile.
- Installation, update, rollback, status, logs, API-outage behavior, and
  security rationale are documented under `docs/`.
- No documentation refers to the template application, extension-service
  deployment, a DaemonSet, or mutable production image tags.
- The disposable VM and all generated sensitive or bulky artifacts are removed.
- Evidence is recorded in the bound session notes and the worktree is clean
  after committing and pushing.

## 6. Final integration gate

Before opening the implementation pull request:

1. Run the repository's normal format, generated-code freshness, unit,
   integration, lint, docs, GoReleaser snapshot, local image, and security
   validation tasks.
2. Confirm generated mocks and lockfiles are current and committed.
3. Search tracked source, configuration, workflows, and docs for template
   identity and obsolete deployment claims; every remaining occurrence must be
   historical journal evidence or removed.
4. Compare `deploy/talos/incus-guest-agent.yaml.tmpl` with the exact manifest used
   in the successful production live test.
5. Inspect changed workflows for pinned actions, minimal permissions, artifact
   digest handoffs, signing, provenance, SBOM, and Trivy behavior.
6. Inspect the production image index and entrypoint one final time.
7. Open a pull request with a Conventional Commit title. Integration happens by
   squash merge on GitHub, never by local merge.

Implementation is complete only when the PR contains the runtime, image,
release cutover, canonical Talos patch, operator documentation, and
production-image live evidence as one coherent product change.