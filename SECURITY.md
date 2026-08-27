# Security Policy

## Supported versions

This project does not publish a multi-version support window. When reporting a
problem, include the exact release version and OCI digest that you tested.

## Verifying release artifacts

Release assets and the OCI image carry SLSA provenance signed by the pinned
`meigma/release` reusable workflow. Because that signing identity lives in the
`meigma/release` repository, `gh attestation verify` requires
`--signer-repo meigma/release`; without it, the command fails with
`Error: verifying with issuer "sigstore.dev"`.

Verify the OCI image by its index digest:

```sh
gh attestation verify \
  oci://ghcr.io/componere/incus-guest-agent@sha256:<digest> \
  --repo componere/incus-guest-agent --signer-repo meigma/release
```

Verify a downloaded release asset:

```sh
gh attestation verify incus-guest-agent_<version>_linux_amd64.tar.gz \
  --repo componere/incus-guest-agent --signer-repo meigma/release
```

To verify the checksum manifest without `gh`, use the `checksums.txt.sigstore.json`
release asset with cosign:

```sh
cosign verify-blob checksums.txt --bundle checksums.txt.sigstore.json \
  --certificate-oidc-issuer https://token.actions.githubusercontent.com \
  --certificate-identity-regexp \
  'meigma/release/\.github/workflows/go-pre-publish\.yml'
```

## Reporting a vulnerability

Report vulnerabilities through
[GitHub private vulnerability reporting](https://github.com/componere/incus-guest-agent/security/advisories/new).

Do not disclose a vulnerability in a public issue, pull request, discussion, or
chat channel.

Include:

- the affected release version, commit, and OCI digest;
- the Talos and Incus versions;
- the affected runtime path;
- reproduction steps or a minimal proof of concept;
- the security impact; and
- relevant logs or suggested mitigations.

Do not include production credentials, certificates, or private keys in the
report.

