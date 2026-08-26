# Implementation Plan: `incus-guest-agent`

Status: produced by a planner agent from ARCHITECTURE.md (session 001), under a zero-design-drift constraint. Design authority remains ARCHITECTURE.md; ambiguities are represented as gates in §2, not resolved.

## 1. Authority, scope, and current baseline

This plan implements `.wt/journal-jmgilman/.journal/001/ARCHITECTURE.md` exactly, in the mandatory delivery order from §7. The spike facts and repository rules summarized in `local://spike-context.md` are treated as established evidence and are not re-researched.

The current repository is still the `meigma/template-go` scaffold:

- `cmd/template-go/` and `internal/{cli,config,templateinfo}/` contain the placeholder Cobra/Viper application.
- `go.mod` still declares `github.com/meigma/template-go` and carries Cobra/Viper dependencies.
- `extension/` does not exist.
- `mise.toml`, `moon.yml`, `.goreleaser.yaml`, `melange.yaml`, `apko.yaml`, `.github/workflows/release.yml`, and `.github/workflows/security-scan.yml` still describe the template application and its Melange/apko image.
- The release workflow already delegates signing, SBOM, provenance, OCI publication, and GitHub Release publication to the full-SHA-pinned `meigma/release` reusable workflows.
- Documentation is the placeholder MkDocs site under `docs/`.

### Fixed product boundaries

The implementation must not add runtime configuration, flags other than `--version`, endpoints, health/readiness, metrics, bundled `incus-agent`, alternate media sources, internal child restart, Image Factory support, or any deferred §7 feature. The final production packages are only:

- `cmd/incus-guest-agent`
- `internal/agent`
- `internal/agent/mocks`
- `internal/linux`

The final extension payload is only:

```text
/manifest.yaml
/rootfs/usr/local/lib/containers/incus-guest-agent/incus-guest-agent
/rootfs/usr/local/etc/containers/incus-guest-agent.yaml
```

Verification below deliberately uses targeted builds, targeted package tests, image validation, publisher dry-runs, and live scenarios. It does not schedule formatters, linters, or a project-wide test suite.

## 2. Decision gates and ambiguities that must not be resolved silently

1. **Builder selection is deliberately deferred.** Attempt a pinned Sidero `bldr` first. Fall back to one short pinned `crane` or `regctl` composition only if `bldr` cannot produce the existing publisher’s required artifact shape. The architecture does not rank `crane` versus `regctl`; choose the smaller option that satisfies the already-proven publisher contract. Do not retain two builder paths. A Go `go-containerregistry` assembler is last resort only. Because the approved final package list gives no home for such an assembler, stop for design-authority approval before adding any `cmd/extension-image`, `internal/oci`, or other package if that last resort is reached.
2. **Builder-specific persistent files are unspecified.** The approved tree names only `extension/manifest.yaml.tmpl` and `extension/incus-guest-agent.yaml`. During the vertical spike, prefer a Moon task invoking the selected builder without adding an unapproved persistent recipe. If the successful builder requires a committed recipe, obtain design-authority approval for its exact path rather than silently adding `extension/pkg.yaml`, a script directory, or another configuration surface.
3. **Tool versions are unspecified.** Select reviewed, mutually compatible versions of the chosen builder, `extensions-validator`, and Mockery during implementation; pin them in `mise.toml` through verifying backends and regenerate all four platform entries in `mise.lock`. Do not use unpinned downloads or `go install`.
4. **Publisher dry-run mechanics and artifact layout are owned by the existing reusable publisher.** Phase 1 must discover and prove the exact handoff using the currently pinned `meigma/release` revision. Do not invent a second publisher interface or workflow. Use the existing publication-disabled/dry-run mode and preserve its artifact ID, artifact digest, and image digest contract.
5. **The §2 file list names device, stage, and process adapters but lists only `stage.go` and `process.go`; it also gives no file for the production `Waiter`.** To avoid adding an unapproved file, keep distinct `DeviceFinder` and `StageManager` adapter types in `internal/linux/stage.go`, and wire a tiny cancelable standard-library `Waiter` implementation in `cmd/incus-guest-agent/main.go`. If a separate `internal/linux/device.go` is desired, treat that as a file-layout clarification only; do not add a package.
6. **The exact `AgentProcess` method/error shape is unspecified.** Use the smallest interface that lets the core observe start failure versus exit after a successful start, assert the fixed CWD, forward SIGTERM/SIGINT, and wait/reap. At most one narrow typed error may distinguish post-start child exit from preparation/start failure. Do not create a hierarchy of process or attempt types.
7. **Multi-platform validator coverage is an explicit gate.** Add exactly one amd64+arm64 index assertion only if the selected `extensions-validator` invocation does not already prove the index contents.
8. **Scratch-image scanner visibility is an explicit gate.** Scan the final extension image if the existing scanner sees its contents; otherwise scan the exact canonical release binaries placed into the image. Do one of these, not both.
9. **Mount propagation may be adjusted only in Phase 1.** If the declared `/dev` and `/run` bind propagation does not permit iso9660/tmpfs mounts and `/dev/incus/sock`, make the smallest evidence-backed change to `extension/incus-guest-agent.yaml`, rebuild, and repeat every Phase 1 gate. If success requires changing CD-ROM-only discovery, the package/port design, or another fixed contract, stop for design review.
10. **Two metadata details are not fully specified:** the literal manifest author string and the exact non-version OCI annotation set. Use the repository’s approved owner identity for `metadata.author`; render the Release Please version into `metadata.version` and the standard OCI version annotation. Preserve only publisher-required title/description/source/license annotations rather than inventing more metadata.
11. **The live nonce outcome is fixed, but the architecture does not name the guest-side Unix-socket request utility or the disposable sandbox image lifecycle commands.** The runbook must use an already-approved temporary sandbox/debug mechanism to GET `/1.0/config/user.<key>` through `/dev/incus/sock`; it must not ship that utility in the extension. Record the exact working command in the runbook after Phase 1 proves it.

## 3. Dependency and parallelization model

The four phases are sequential gates: Phase 2 cannot start until both release blockers and the nonce path pass in Phase 1; Phase 3 cannot remove Melange/apko until the Phase 1 hello-payload gate passes; Phase 4’s release rehearsal depends on the completed Phase 3 workflow.

Within those constraints:

- **Phase 1:** the minimal binary/service work and tool pin investigation can proceed in parallel, but builder validation needs both; the sandbox run needs a validated/installable image.
- **Phase 2:** after `internal/agent/ports.go` fixes the four interfaces, core state-machine work and Linux adapter work are independent. Mock generation follows the port definitions. T1 tests can be written with their owning code; T2 core tests follow generated mocks.
- **Phase 3:** GoReleaser trimming and release/security workflow wiring can proceed in parallel once the chosen builder’s artifact contract is known. Melange/apko deletion is last.
- **Phase 4:** the install how-to and sandbox runbook can be drafted in parallel. The full rehearsal starts only after both accurately describe the Phase 3 artifact and live Phase 1/2 commands.

# Phase 1 — Vertical spike: prove the two release blockers first

## Goal

Prove, with the final service mount declaration and a minimal static wrapper, that:

1. a machined extension container can mount the Incus iso9660 media and the `/run/incus_agent` tmpfs through the declared propagation;
2. the resulting two-platform Talos extension passes `extensions-validator` and the existing publisher dry-run; and
3. `/dev/incus/sock` supports the nonce round-trip on `sandbox01`.

A spike implementation may be direct and disposable. It must not survive the Phase 2 cutover as a second runtime path.

## Ordered steps

### 1.1 Create the minimum final payload metadata

Create:

- `extension/manifest.yaml.tmpl`
- `extension/incus-guest-agent.yaml`

`extension/manifest.yaml.tmpl` must contain only the approved extension metadata:

- `version: v1alpha1`
- name `incus-guest-agent`
- Release Please-rendered version placeholder
- approved author and a restrained description of the Incus guest socket service
- `compatibility.talos.version: ">= v1.13.9"`

`extension/incus-guest-agent.yaml` must initially be the exact §4 service:

- dependencies only on `/system/run/machined/machine.sock` and `/dev/vsock`
- entrypoint `./incus-guest-agent`
- `/dev` bind mounted to `/dev` with `rshared,rbind,rw`
- `/run` bind mounted to `/run` with `rshared,rbind,rw`
- `restart: always`
- no `/dev/sr0` dependency

Check the rendered payload contains no base image files, packages, shell, extra service, or extra mount.

### 1.2 Build a minimal static spike wrapper

Create `cmd/incus-guest-agent/main.go` as the spike composition and behavior in one file. It must be sufficient to exercise the real path:

- support only `--version` plus normal no-argument service execution;
- create the owned media/stage directories;
- lexically enumerate block devices matching `/dev/sr*`;
- mount candidates read-only as `iso9660` with `nosuid,nodev,noexec` at `/run/incus-guest-agent/media`;
- require the five regular files and an executable `incus-agent`;
- mount a `mode=0700,size=50M` tmpfs at `/run/incus_agent`;
- stream-copy exactly the five files while preserving their permission bits;
- unmount the media and run `./incus-agent` with CWD `/run/incus_agent` and inherited stdio;
- retry with `1,2,4,8,16,30,30…` second delays until canceled;
- forward SIGTERM/SIGINT, wait for the child, clean owned mounts, return zero for requested shutdown, and return non-zero for an unsolicited child exit.

Build the actual spike binary, not a hello-world substitute:

```sh
CGO_ENABLED=0 GOOS=linux GOARCH=amd64 go build -trimpath -o <amd64-output> ./cmd/incus-guest-agent
CGO_ENABLED=0 GOOS=linux GOARCH=arm64 go build -trimpath -o <arm64-output> ./cmd/incus-guest-agent
```

Inspect both outputs to confirm they are Linux binaries for the requested architecture and have no dynamic runtime dependency. Run the host-compatible build with `--version` and confirm it exits zero without starting the service.

### 1.3 Execute the builder selection gate

Modify:

- `mise.toml`
- `mise.lock`
- `moon.yml`

First add pinned `bldr` and `extensions-validator` tools to `mise.toml` through verifying backends, regenerate `mise.lock` for `linux-x64,linux-arm64,macos-x64,macos-arm64`, and add a Moon extension-image task that consumes the two canonical binaries, renders `extension/manifest.yaml.tmpl`, and creates the exact per-platform payload.

Run the selected task through Moon. For each platform, inspect the extracted payload and modes. It must contain exactly:

```text
manifest.yaml
rootfs/usr/local/lib/containers/incus-guest-agent/incus-guest-agent       0755
rootfs/usr/local/etc/containers/incus-guest-agent.yaml                    0644
```

Then run, for each platform payload:

```sh
extensions-validator validate --rootfs=<extracted-platform-payload> --pkg-name=incus-guest-agent
```

Decision sequence:

1. Keep `bldr` only if both payloads validate and its OCI layout satisfies the existing publisher handoff.
2. If it cannot satisfy the publisher artifact shape, remove the `bldr` pin/task cleanly and replace it with one pinned `crane` or `regctl` composition; repeat payload inspection and both validator runs.
3. If neither option works, stop before adding a Go assembler and return to the design authority.

Do not remove `melange.yaml`, `apko.yaml`, their mise pins, or the current `image-local` task yet.

### 1.4 Prove the existing publisher dry-run

Temporarily exercise `.github/workflows/release.yml` against the selected builder’s OCI artifact while keeping both publication inputs disabled. Preserve the existing full-SHA reusable workflow revision, permissions model, and publisher outputs. The dry-run must accept the builder output, resolve the final image digest, and complete all prepare/verification steps without pushing an image or publishing a GitHub Release.

This is the hello-payload gate:

- amd64 validator passes;
- arm64 validator passes;
- publisher dry-run passes with the selected OCI artifact and digest.

Only after all three pass is the builder selected. Retain the selected Moon/mise path; remove temporary workflow experimentation that is not part of the Phase 3 integration.

### 1.5 Prove mount propagation and nonce behavior on `sandbox01`

Publish the validated spike image only to an approved disposable sandbox reference, configure a disposable Talos v1.13.9 VM to install that raw OCI reference, and boot it **without** an `agent:config` device.

Gate the following sequence:

1. Confirm `incus-guest-agent` is running and logs one warning per failed sweep while remaining in retry; it must not require a service restart.
2. Hot-add the CD-ROM from the Incus host:

   ```sh
   incus config device add <vm> agent disk source=agent:config
   ```

3. Confirm service logs show a successful iso9660 mount, five-file stage, media unmount, tmpfs stage, and child start. Confirm `/dev/incus/sock` appears without restarting the service.
4. Set a unique nonce on the host:

   ```sh
   incus config set <vm> user.<nonce-key>=<unique-value>
   ```

5. Through the already-approved guest-side debug mechanism, GET `/1.0/config/user.<nonce-key>` over `/dev/incus/sock`; require the exact unique value.
6. Restart only the machined service:

   ```sh
   talosctl -n <vm-ip> service incus-guest-agent restart
   ```

7. Require `/dev/incus/sock` to return and repeat the nonce GET successfully.

If mount propagation fails, change only `extension/incus-guest-agent.yaml` as allowed by decision gate 9, rebuild, and repeat steps 1.3–1.5. Clean up the disposable VM, device, and sandbox image reference after evidence is captured.

## Phase 1 risks and mitigations

- **`bldr` cannot satisfy the existing publisher layout:** follow the mandated fallback order; do not design a custom assembler prematurely.
- **Validator accepts per-platform payloads but not the assembled index:** record the coverage gap for Phase 3’s single conditional index assertion.
- **Scratch payload is invisible to the vulnerability scanner:** record the result for Phase 3’s binary-scan fallback; do not add a second scan.
- **Mount propagation differs inside machined:** adjust only the service mount contract during this phase and rerun every gate.
- **False-positive nonce proof from stale state:** use a unique nonce, begin without the device, require hot-add without restart, then require a second read after service restart.

## Phase 1 acceptance criteria

- One selected, pinned builder path remains, chosen in the required order.
- Both platform payloads contain exactly the three approved paths with modes 0755/0644 and no base image content.
- `extensions-validator` passes for amd64 and arm64.
- The existing publisher dry-run accepts the artifact and resolves its digest with publication disabled.
- On `sandbox01`, retry-before-media, hot-add without restart, iso9660/tmpfs mounting, `/dev/incus/sock`, nonce read, and nonce read after service restart all pass.
- Any mount-contract adjustment is evidence-backed and confined to the service YAML.

# Phase 2 — Production cutover

## Goal

Replace the direct spike and all template application code with the approved two-package hexagonal implementation, four generated mocks, and the complete T1/T2 matrix. There must be one runtime path only.

## Ordered steps

### 2.1 Replace project identity and placeholder dependencies

Modify:

- `go.mod`
- `go.sum`

Change the module to `github.com/componere/incus-guest-agent`. Remove Cobra/Viper and their now-unused transitive dependencies. Add only dependencies required by the approved implementation and tests: `golang.org/x/sys` if used for Linux mount/process syscalls, Testify for tests/generated mocks, and no additional runtime framework.

Do not add Cobra, Viper, a config loader, or a logging/metrics dependency. Use the standard library for `--version`, signals, process execution, copying, time, and logging.

### 2.2 Define the consumer-owned core ports

Create:

- `internal/agent/doc.go`
- `internal/agent/ports.go`

Define and document exactly:

- `type DevicePath string`
- `type DeviceFinder interface`
- `type StageManager interface`
- `type AgentProcess interface`
- `type Waiter interface`

The method shapes must express only the §2 contracts. Paths, staged filenames, mount options, and retry limits are compile-time constants, not options or configuration. Do not add `Attempt`, generalized cleanup/error aggregation, configuration structs, readiness, or metrics interfaces.

Fix the `AgentProcess` shape at this point so it can distinguish start failure from exit after successful start, accept/verify the fixed staged execution location, forward signals, and wait. If a typed error is necessary, add only the one narrow post-start child-exit distinction permitted by the architecture.

### 2.3 Implement the side-effect-free core state machine

Create `internal/agent/agent.go` and replace the spike control flow in `cmd/incus-guest-agent/main.go` with a call into the core.

`internal/agent/agent.go` must own:

- enumerate → try candidates in lexical order → stage first valid candidate → start → wait;
- `1,2,4,8,16,30,30…` second capped retry with no jitter;
- cancellation during wait;
- cleanup-before-next-candidate and never-start-after-failed-stage invariants;
- retry after absent media, a failed sweep, preparation failure, or start failure;
- no internal restart after an unsolicited child exit;
- one warning per failed sweep and debug-only per-candidate misses;
- zero result on requested shutdown after forwarding/waiting/cleanup;
- non-zero/error result on unsolicited child exit after cleanup.

The core must use only the four ports plus standard data/log values; it must perform no filesystem, mount, process, signal, or timer side effect directly.

`cmd/incus-guest-agent/main.go` becomes the final composition root:

- linker-injected `version`, `commit`, and `date` values;
- dependency-free `--version` handling;
- standard stderr logging;
- SIGTERM/SIGINT subscription;
- construction of the Linux adapters and the tiny cancelable standard-library `Waiter`;
- mapping requested shutdown to exit 0 and preparation/start or unsolicited child-exit failure to non-zero.

Delete every spike-only private implementation from `main.go`; do not keep a fallback path.

### 2.4 Implement the Linux adapters

Create:

- `internal/linux/doc.go`
- `internal/linux/stage.go`
- `internal/linux/process.go`

In `stage.go`, keep separate concrete adapters for `DeviceFinder` and `StageManager` even though they share a file:

- `DeviceFinder` globs `/dev/sr*`, filters to block devices, converts to `agent.DevicePath`, and returns a lexical sort; it never mounts.
- `StageManager` creates owned directories once, mounts each candidate at `/run/incus-guest-agent/media` as read-only `iso9660` with `nosuid,nodev,noexec`, validates/copies exactly `incus-agent`, `agent.conf`, `agent.crt`, `agent.key`, and `server.crt`, requires all five to be regular and the agent to have an execute bit, mounts `/run/incus_agent` as tmpfs with `mode=0700,size=50M`, streams copies, preserves source permission bits only, and owns teardown of its media/tmpfs mounts.
- A post-stage media unmount failure is logged and retained for shutdown retry but does not block child start.
- Any other preparation failure restores owned state before returning.
- There is no recursion, symlink following, full-tree validation, legacy `lxd-agent` alias, eject operation, or copy of install scripts.

In `process.go`, implement the `AgentProcess` adapter with:

- executable `./incus-agent`;
- CWD `/run/incus_agent`;
- no arguments and no shell;
- inherited stdin/stdout/stderr;
- direct-child start, SIGTERM/SIGINT forwarding, wait/reaping, and exit reporting.

Keep process supervision separate from stage/mount ownership.

### 2.5 Pin Mockery and generate exactly four mocks

Create:

- `.mockery.yaml`
- `internal/agent/mocks/doc.go`
- `internal/agent/mocks/device_finder.go` (generated)
- `internal/agent/mocks/stage_manager.go` (generated)
- `internal/agent/mocks/agent_process.go` (generated)
- `internal/agent/mocks/waiter.go` (generated)

Modify:

- `mise.toml`
- `mise.lock`
- `moon.yml`

Pin Mockery through a verifying mise backend, lock all four supported tool platforms, and configure `.mockery.yaml` to select only the four interfaces in `internal/agent`, emit deterministic snake-case files into the `mocks` subpackage, and use the Testify-based generated constructors. Add a Moon mock-generation task; commit generated output. Production packages must never import `internal/agent/mocks`.

Generate through the pinned task and rerun it after generation; the second run must produce no file changes.

### 2.6 Add the exact T1/T2 test matrix

Create:

- `internal/agent/agent_test.go` — T1 retry/cancellation unit behavior.
- `internal/agent/agent_integration_test.go` — T2 core behavior through the four generated port mocks.
- `internal/linux/stage_test.go` — T1 five-file helper and filesystem adapter behavior in temporary directories.
- `internal/linux/process_test.go` — T2 process CWD, inherited execution behavior, signal forwarding, wait, and reaping with a helper child.

Required cases:

**T1 — `internal/agent/agent_test.go`**

- delay sequence reaches `1,2,4,8,16,30` and stays at 30;
- cancellation interrupts a pending wait and prevents another sweep.

**T1 — `internal/linux/stage_test.go`**

- valid five-file input copies exactly those files and preserves permission bits;
- each missing required file fails;
- a directory, symlink, or other non-regular required entry fails;
- `incus-agent` without any execute bit fails;
- a deterministic destination/copy error fails and leaves no usable partial stage;
- filesystem cleanup behavior is exercised only with `t.TempDir()`; do not create pretend privileged mount fixtures.

**T2 — `internal/agent/agent_integration_test.go`**

- no media retries until canceled;
- an invalid first candidate is cleaned before the next candidate;
- preparation failure or process-start failure retries and never produces a partial start;
- success uses the fixed CWD and distinguishes unsolicited child exit from requested shutdown.

Assert order only for safety invariants: cleanup before the next candidate and never start after failed staging. Do not assert incidental call order.

**T2 — `internal/linux/process_test.go`**

- a helper child observes `/run/incus_agent` as its requested CWD contract;
- SIGTERM/SIGINT is forwarded;
- the adapter waits/reaps and reports the child exit correctly.

Use Testify assertions and Mockery-generated mocks; do not hand-write mocks or fakes for the four ports.

### 2.7 Delete the template implementation cleanly

Delete:

- `cmd/template-go/main.go`
- `internal/cli/root.go`
- `internal/cli/root_test.go`
- `internal/config/config.go`
- `internal/templateinfo/info.go`
- `internal/templateinfo/info_test.go`

Remove the now-empty directories. Remove all imports and references to `github.com/meigma/template-go/internal/{cli,config,templateinfo}`. Do not leave aliases, re-exports, deprecated paths, or compatibility shims.

### 2.8 Targeted verification and live regression

Run only the affected package tests and binary smoke:

```sh
go test ./internal/agent
go test ./internal/linux
go build -o <smoke-output> ./cmd/incus-guest-agent
<smoke-output> --version
```

Then rebuild the extension through the selected Moon task, which must rerun both platform validators, install the production image into a new disposable `sandbox01` VM, and repeat the Phase 1 live sequence: retry without media, hot-add, socket without restart, nonce, service restart, nonce again.

## Phase 2 risks and mitigations

- **Core accidentally owns I/O:** enforce that `internal/agent` imports no OS mount/process/time implementation and exercises all external behavior through the four ports.
- **Generated-mock import cycle:** keep production interfaces in `internal/agent`, generated mocks in `internal/agent/mocks`, and mock-driven tests in the external `agent_test` package where necessary.
- **Signal race or unreaped child:** prove it with the helper-child test and the live service restart; do not use `syscall.Exec`.
- **Partial stage after copy failure:** make stage construction transactional and test the failed destination path.
- **Unit tests falsely simulate mount truth:** keep mount propagation exclusively in the live T3 scenario.

## Phase 2 acceptance criteria

- The final tree has only the approved runtime packages and four ports.
- The template command/packages and Cobra/Viper dependencies are gone with no shims.
- All four mocks are generated reproducibly by pinned Mockery.
- Every specified T1/T2 behavior passes in the four named test files.
- The final binary has only `--version`, is statically built for Linux, forwards signals, and leaves restart to machined.
- The production image passes both validators and repeats all three live T3 conditions on `sandbox01`.

# Phase 3 — Release integration

## Goal

Make the proven builder the canonical two-platform extension build, place the validator/index/scan gates before the unchanged publisher chain, trim GoReleaser to the approved static Linux artifacts, and remove Melange/apko only after the hello-payload gate remains green.

## Ordered steps

### 3.1 Trim GoReleaser to the extension product

Modify `.goreleaser.yaml`:

- rename the project/build/binary to `incus-guest-agent` and point `main` at `./cmd/incus-guest-agent`;
- keep `CGO_ENABLED=0`, `-trimpath`, the existing reproducible build flags, and version/commit/date ldflags;
- restrict builds to Linux `amd64` and `arm64`;
- retain archives/checksums, SBOM generation, checksum Sigstore bundle, `gomod.proxy`, and release-disabled handoff used by the reusable release workflow;
- remove Darwin, Windows, notarization, Windows format overrides, nfpms, Homebrew casks, and Scoop output.

Validate only this release contract:

```sh
goreleaser check
goreleaser release --snapshot --clean
```

Inspect the snapshot to confirm there are exactly two Linux architecture builds and no native packages, Darwin/Windows artifacts, casks, or Scoop manifests.

### 3.2 Make the selected builder canonical in Moon/mise

Modify:

- `moon.yml`
- `mise.toml`
- `mise.lock`

Finalize Moon file groups and tasks so extension source inputs include the GoReleaser outputs, `extension/manifest.yaml.tmpl`, `extension/incus-guest-agent.yaml`, and the selected builder configuration, while release configuration inputs point at the actual `.github/workflows/*.yml` rather than the unused `.github/workflows.disabled/**/*.yml` placeholder.

The extension build task must:

1. consume the canonical GoReleaser Linux binaries;
2. render the Release Please version into `manifest.yaml` and the OCI version annotation;
3. create amd64 and arm64 platform payloads;
4. assemble one OCI index using the selected builder;
5. invoke `extensions-validator` for each platform on every image build.

Remove the apko-specific `[tasks.image-local]` task from `mise.toml`; the extension image is built through Moon. Keep only the selected builder, validator, Mockery, and the existing release/security tools.

### 3.3 Wire one release gate per responsibility

Modify `.github/workflows/release.yml` while retaining its tag trigger, concurrency, minimal permissions, full-SHA-pinned `meigma/release` revision, signing/SBOM/provenance publisher, and release-app contract.

Order jobs by `needs` as follows:

1. **`release-assets`** — existing Go pre-publish job produces canonical assets and Linux OCI inputs.
2. **`extension-image`** — selected builder creates the two platform payloads and OCI index and publishes the workflow artifact outputs expected by the existing publisher.
3. **`extension-validate`** — one matrix job validates amd64 and arm64 payloads with `extensions-validator`.
4. **`extension-index`** — add this single assertion job only if Phase 1 proved the validator does not check that the index contains exactly `linux/amd64` and `linux/arm64`.
5. **`vulnerability-scan`** — scan the final OCI image/layout, or the exact two canonical binaries if Phase 1 proved scratch content is invisible. Fail on the repository’s existing HIGH/CRITICAL fixed-vulnerability policy.
6. **`oci-publish`** — existing reusable publisher consumes the already validated artifact/digest.
7. **`github-release`** — existing reusable publisher waits for release assets and OCI publication/dry-run and publishes the GitHub Release only when enabled.

Keep `publish-image: false` and `publish-release: false` until the Phase 4 rehearsal passes. Do not duplicate signing, SBOM, or provenance work in the new builder/validator jobs.

### 3.4 Replace the scheduled Melange/apko scan path

Modify `.github/workflows/security-scan.yml`:

- replace the inline `cmd/template-go` + Melange + apko build with the selected canonical extension build;
- use the same version/commit/date injection and payload as the release workflow;
- scan the final image or binary fallback chosen by the Phase 1 visibility gate;
- retain the existing Trivy policy, SARIF upload, minimal permissions, and full-SHA action pins;
- rename image and SARIF metadata from `template-go` to `incus-guest-agent`.

Do not create a second security workflow or add a second scan target.

### 3.5 Preserve Release Please as the sole version source

Modify `release-please-config.json` to rename the package to `incus-guest-agent`. Keep `.release-please-manifest.json` as the version state; do not create another version file. The extension build must render that released version into:

- `extension/manifest.yaml.tmpl` → `/manifest.yaml` `metadata.version`;
- the OCI version annotation;
- the GoReleaser ldflags already used by `main.version`.

Leave `.github/workflows/release-please.yml` behavior unchanged.

### 3.6 Remove the obsolete image toolchain only after all preceding gates pass

Delete:

- `melange.yaml`
- `apko.yaml`

Remove the Melange/apko pins from `mise.toml` and regenerate `mise.lock` for all four platforms. Do this only after a fresh two-platform extension build, both validator runs, and publisher dry-run pass with the selected builder. Confirm `.github/workflows/`, `moon.yml`, and `mise.toml` contain no Melange/apko commands or `template-go` image names.

### 3.7 Targeted release verification

Run:

```sh
goreleaser check
goreleaser release --snapshot --clean
moon run root:<extension-build-task>
```

Then run both validator invocations, the conditional index assertion if required, the chosen single vulnerability scan, and the existing publisher in publication-disabled mode. Inspect outputs rather than running the root aggregate suite.

## Phase 3 risks and mitigations

- **Builder output drifts from publisher inputs:** reuse only the artifact shape proven in Phase 1 and keep publisher dry-run as a required gate.
- **Release assets are rebuilt independently for the image:** require the extension builder to consume the canonical GoReleaser Linux bytes; do not compile again inside the image job.
- **Version divergence:** accept version only from Release Please/GoReleaser context and render it into both manifest and annotations.
- **Supply-chain regression:** leave the existing signing, checksum bundle, SBOM, provenance, permissions, and reusable workflow SHA unchanged.
- **Premature Melange/apko removal:** delete them last, only after the replacement passes validators and publisher dry-run.

## Phase 3 acceptance criteria

- GoReleaser emits only static Linux amd64/arm64 release builds plus the retained archive/checksum/SBOM/signature handoff.
- Every extension image build validates both platform payloads.
- The final index contains exactly amd64 and arm64; there is no redundant assertion if the validator already proves this.
- Exactly one vulnerability scan covers the final shipped bytes.
- The existing sign/SBOM/provenance/publish chain remains intact and succeeds in dry-run mode.
- Release Please is the only version source and the binary, manifest, and OCI annotation agree.
- Melange/apko files, pins, tasks, and workflow commands are gone only after the replacement gate passes.

# Phase 4 — Documentation and full rehearsal

## Goal

Replace template documentation with the raw-OCI installation how-to and the manual sandbox e2e runbook, then execute one release-faithful rehearsal before enabling v1 publication.

## Ordered steps

### 4.1 Write the raw-OCI install how-to

Create:

- `docs/docs/how-to/install.md`

The how-to must state:

- Talos compatibility is `>= v1.13.9` and must not be widened without evidence;
- users install the raw OCI extension reference through `machine.install.extensions`;
- every Talos VM must have the Incus device:

  ```sh
  incus config device add <vm> agent disk source=agent:config
  ```

- the device may be hot-added after boot and the retry loop discovers it without service restart;
- success means `/dev/incus/sock` is available to consumers;
- the extension uses the host-shipped, version-matched `incus-agent` and does not bundle one;
- v1 is CD-ROM-only;
- the known mount-namespace reporting and host-side `incus exec` rootfs limitations are cosmetic/accepted;
- Image Factory schematics and every other §7 deferred feature are out of scope.

Do not document flags, environment variables, configuration files, health endpoints, or alternate media paths.

### 4.2 Write the manual T3 runbook

Create:

- `docs/docs/how-to/rehearse-sandbox01.md`

Document the exact working commands learned in Phases 1–2 for:

1. creating/booting a disposable Talos VM with the candidate raw OCI extension and no `agent:config` device;
2. checking the service/logs remain in retry;
3. hot-adding `source=agent:config`;
4. checking `/dev/incus/sock` appears without service restart;
5. setting a unique `user.*` nonce and reading the exact value through `/1.0/config/user.<key>` over the Unix socket;
6. restarting `incus-guest-agent` through `talosctl` and repeating the nonce read;
7. cleaning up the VM/device/disposable image.

Keep the runbook manual. Do not add e2e automation in v1; future automation is limited to the same three pass conditions: retry/hot-add socket, nonce round-trip, and nonce after service restart.

### 4.3 Replace documentation/template identity

Modify:

- `docs/docs/index.md`
- `docs/mkdocs.yml`
- `docs/moon.yml`
- `docs/pyproject.toml`
- `docs/uv.lock`
- `README.md`

Update project/repository identity to `componere/incus-guest-agent`, add only the two how-to pages to MkDocs navigation, and make the README a concise project overview that links to the user documentation. Regenerate `docs/uv.lock` after renaming the docs project metadata; do not change documentation dependencies.

Delete:

- `DELETE_ME.md`

Do not manually rewrite historical `CHANGELOG.md`; Release Please owns future changelog entries. Do not add empty tutorial/reference/explanation scaffolds.

Verify the documentation only:

```sh
moon run docs:build --summary minimal
```

The strict MkDocs build must pass with all navigation links resolved.

### 4.4 Run one full release-faithful rehearsal before v1

Use the existing Release Please path to create a pre-v1 candidate while `.github/workflows/release.yml` still has `publish-image: false` and `publish-release: false`.

Require the real workflow sequence to complete:

- GoReleaser canonical assets;
- selected two-platform extension builder;
- amd64 and arm64 validators;
- conditional index assertion only if required;
- the selected single vulnerability scan;
- existing checksum signature, SBOM, provenance, and attestation generation;
- publisher dry-run and populated draft release artifacts without public publication.

Verify the candidate’s binary version, `manifest.yaml` version, OCI version annotation, checksums, SBOM, attestation/provenance subjects, and image digest all refer to the same Release Please version and shipped bytes.

Install that candidate via its raw OCI reference or approved disposable rehearsal reference on `sandbox01` and execute the runbook end to end. Require the three T3 pass conditions exactly:

1. service retries without media and hot-add produces `/dev/incus/sock` without service restart;
2. a unique `user.*` nonce round-trips;
3. the nonce round-trips again after `talosctl` service restart.

After, and only after, the workflow and live rehearsal both pass, change `publish-image` and `publish-release` to `true` in a reviewed `.github/workflows/release.yml` change for the subsequent v1 release. The rehearsal itself must not publish v1.

## Phase 4 risks and mitigations

- **Docs accidentally promise deferred behavior:** restrict instructions to raw OCI, Talos v1.13.9+, CD-ROM, the required Incus device, and the socket contract.
- **Runbook cannot reproduce the nonce request:** record the exact Phase 1 guest-side debug command rather than inventing or shipping a helper.
- **A “dry-run” skips the real artifact path:** use the same Release Please tag/draft, builder, validators, scanner, publisher preparation, signatures, SBOM, and provenance jobs as the real release; disable only public publication.
- **Rehearsal validates a stale image:** use a unique candidate version/digest and a unique nonce, and compare versions across binary, manifest, annotations, and workflow artifacts.

## Phase 4 acceptance criteria

- The strict docs build passes.
- The install how-to states the raw OCI path and mandatory `incus config device add <vm> agent disk source=agent:config` requirement.
- The manual runbook contains the exact proven `sandbox01` commands and only the approved three future automation conditions.
- Template-facing README/docs metadata and `DELETE_ME.md` are removed/replaced; no placeholder runtime code remains.
- A release-faithful, publication-disabled candidate completes build, validation, scan, signing, SBOM, provenance, publisher, and draft-release steps.
- The candidate passes all three live T3 conditions on `sandbox01`.
- Public image/release inputs are enabled only in a reviewed change after the rehearsal, leaving v1 ready without publishing it as part of the rehearsal.

# Final completion gate

The implementation is ready for v1 only when all four phase gates are satisfied in order, the final tree contains no alternate builder/runtime path or template shim, and every fixed §1–§6 contract in `ARCHITECTURE.md` is represented by either targeted T1/T2 evidence, per-platform validator evidence, publisher dry-run evidence, or the manual `sandbox01` T3 evidence.