---
id: 003
title: Implementation session
started: 2026-08-26
---

## 2026-08-26 17:34 — Kickoff
Goal for the session: start a new working session; no task stated yet.
Current state of the world: session 001 produced spike evidence, approved
ARCHITECTURE.md, and PLAN.md (implementation to begin at Phase 1: vertical
spike — minimal wrapper, final service YAML, builder-selection gate, publisher
dry-run, sandbox01 nonce proof). Session 002 (review of preview artifacts) is
in-progress in parallel. Repo is still the untouched template-go scaffold plus
a `ref/` gitignore entry.
Plan: await the user's request; likely candidates are PLAN.md Phase 1 work.

## 2026-08-26 17:52 — Docs pass complete
Reviewed the full doc set (README, CONTRIBUTING, SECURITY, docs/docs/*) against
diataxis + language-style. Structure was already right-sized (2 how-tos, 1
reference, 1 explanation); kept it. Vetted every runtime claim against source
(const.go, stage.go, files.go, device.go, process.go, main.go, agent.go,
yaml.tmpl, apko.yaml, moon.yml, mise.toml) — all constants, log messages, and
CLI contract were accurate. Fixed: install.md anonymous-pull overclaim
("proves the node can pull" → confirms package resolves anonymously),
"other versions have not passed" → "untested", placeholder unification,
rollback previous-patch provenance, README duplication of runtime.md and
CONTRIBUTING task list (root:check also runs docs:build — README now says so),
kubelet/wrapper actor ambiguity, vague AF_VSOCK bullet, index.md trailing
imperative. Verified with moon run docs:build (mkdocs --strict) — pass.
Branch docs/style-pass, PR #12 open awaiting review.

## 2026-08-26 18:02 — README restructure
Per user follow-up: README now leads with what/why (Talos immutable, no agent
loader → incus info/exec and /dev/incus/sock need the static pod), adds a
minimal attach/render/apply quickstart deferring to the install guide, and all
doc links now target the live GitHub Pages site (URL scheme verified against
the deployed site). Pushed de25a79 to docs/style-pass; PR #12 body updated.

## 2026-08-26 18:07 — PR #12 merged
Squash-merged as dc61cc1 (docs: tighten operator docs for accuracy and style).
Master fast-forwarded, docs/style-pass worktree and branch removed. GitHub
Pages will republish from master. Docs pass complete.

## 2026-08-26 18:25 — Dependabot triage complete
Merged all 5 open Dependabot PRs (#1-#5: actions/cache 6.1.0, mkdocs-material
9.7.7, codeql upload-sarif 4.37.8, mise-action 4.2.5, actions/checkout 7.0.1);
all SHA-pinned with version comments, all CI green. The two security alerts
(pymdown-extensions: GHSA-gm37-52c6-37mw high ReDoS, GHSA-9xwg-3r6f-jcx2
moderate b64 path traversal) were NOT covered by any PR — transitive dep of
mkdocs-material in docs/uv.lock. Fixed via uv lock --upgrade-package
pymdown-extensions (10.21.3 -> 11.0.2), strict docs build verified, PR #13
squash-merged as d1def9b. Both alerts now show state=fixed. Remaining open PR
is release-please 1.0.0 (#9) — intentionally left for the user.

## 2026-08-26 18:40 — Release pinned to 0.1.0
Release-please PR #9 proposed 1.0.0 (default when no release exists; the
bump-minor-pre-major flags only govern later bumps). Added
"initial-version": "0.1.0" to release-please-config.json (verified against
the current upstream config schema), merged as PR #14 (0f3f563). The
push-triggered workflow rewrote PR #9 to "chore(master): release 0.1.0".
PR #9 left open for the user to cut the release.

## 2026-08-26 18:55 — Release pre-flight check
Verified the full v0.1.0 release path (meigma/release pinned at 0dee66f =
v0.1.17): org var COMPONERE_RELEASE_APP_CLIENT_ID + secret
..._PRIVATE_KEY exist (selected visibility; proven readable by the
release-please run), tag ruleset "Default tags" (~ALL, required_signatures)
has the release app (Integration 4551177) as always-bypass, all four reusable
workflow input/output/secret contracts match release.yml, actions policy
allows all. Local rehearsal: goreleaser check OK (ldflags stamp
main.version/commit/date), mise run image-local OK (melange sign -> apko ->
docker run --version/--help, entrypoint /usr/bin/incus-guest-agent, user
65532, arm64). Pin is 4 commits behind meigma/release main (docs + multi-
binary feature only; no fixes needed). GHCR package does not exist yet ->
first publish creates it PRIVATE; post-release must verify repo link + set
public or Talos nodes fail anonymous pulls (CONTRIBUTING documents this).
PR #9 (release 0.1.0) MERGEABLE/CLEAN. Pre-flight: GO.

## 2026-08-26 19:20 — v0.1.0 released
Merged PR #9; release-please created tag v0.1.0 + draft release; the tag
pipeline (go-pre-publish -> go-oci-build -> publish-oci-image ->
publish-github-release) went green in ~4 min. Release published (undrafted)
with checksums.txt(+sigstore), per-arch tarballs + SBOMs. Image
ghcr.io/componere/incus-guest-agent:0.1.0 (index digest sha256:c938c2c6...,
amd64+arm64). Surprise: package was created PUBLIC and auto-linked to the
repo (no manual visibility flip needed — pre-flight assumption of
default-private did not hold for this publish path). Verified: anonymous
manifest fetch HTTP 200, docker run --version prints 0.1.0, provenance
attestation verifies via gh attestation verify.

## 2026-08-26 19:35 — Attestation verification note
Correction to the 19:20 entry: `gh attestation verify` (gh 2.97.0, nixpkgs)
fails locally with a bare `Error: verifying with issuer "sigstore.dev"` for
both the OCI image and release tarballs — no detail even with GH_DEBUG.
GitHub's attestation API returns 1 attestation for the index digest, and
independent verification succeeds: cosign verify-blob of checksums.txt
against checksums.txt.sigstore.json (Fulcio identity meigma/release
go-pre-publish, GitHub OIDC issuer) = Verified OK; amd64 tarball checksum
matches. Conclusion: release chain is sound; the gh failure is a local CLI/
trust-root issue, worth retrying from another gh build before suspecting the
pipeline.

## 2026-08-26 20:05 — gh attestation verify root cause
Not a local/gh-build issue (mise-provided gh 2.98.0 failed identically; TUF
cache clear and clock ruled out). Root cause: the SLSA provenance certs are
signed with the REUSABLE WORKFLOW identity (SAN =
meigma/release/.github/workflows/publish-github-release.yml@0dee66f for
release assets; sourceRepositoryURI = componere/incus-guest-agent), and gh's
default policy requires the signer to live in --repo. Fix: pass
`--signer-repo meigma/release`. Both the amd64 tarball and the OCI index
digest then verify (exit 0, public-good sigstore). Bonus finding: each digest
carries a second attestation signed by GitHub's own Fulcio (SAN
dotcom.releases.github.com, predicate in-toto release/v0.2) — GitHub's
immutable-release attestation. Verification command worth documenting:
  gh attestation verify <artifact|oci://ref@digest> \
    --repo componere/incus-guest-agent --signer-repo meigma/release

## 2026-08-26 20:15 — Verification docs shipped
Added "Verifying release artifacts" to SECURITY.md (PR #15, squash-merged):
gh attestation verify with --signer-repo meigma/release for the OCI image and
release assets, plus the cosign verify-blob path for checksums.txt (confirmed
the deprecated --new-bundle-format flag is unnecessary with the pinned cosign).
All three commands were run against the live v0.1.0 artifacts before
documenting.

## 2026-08-26 20:30 — Close
Session closed. All work merged: PR #12 (docs pass + README restructure),
Dependabot #1-#5, PR #13 (pymdown-extensions 11.0.2, both alerts fixed),
PR #14 (initial-version 0.1.0), PR #9 (release 0.1.0), PR #15 (SECURITY.md
verification docs). v0.1.0 live: release published with signed checksums,
SBOMs, and provenance; image ghcr.io/componere/incus-guest-agent:0.1.0
public and verified (anonymous pull, --version, cosign, gh attestation with
--signer-repo meigma/release). Handoff: next engineering work is the
remaining plan phases per .journal/002/PLAN.md; meigma/release pin is 4
commits behind upstream (non-urgent). SUMMARY.md written; INDEX.md and
TECH_NOTES.md updated.
