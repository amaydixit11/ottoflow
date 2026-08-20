# Creating a Release

This document describes how to create an OttoFlow release. A release produces a versioned Helm chart (published to GHCR as an OCI artifact) and a GitHub Release with the CLI binaries.

## How releases work

1. You push a **version tag** (e.g. `v0.1.0`) to the repository.
2. The **release workflow** (`.github/workflows/release.yaml`) runs on that tag push.
3. The workflow:
   - Validates tag format
   - Sets `charts/ottoflow/Chart.yaml` `version` and `appVersion` from the tag
   - Packages the Helm chart and **pushes it to GHCR (OCI)** at `oci://ghcr.io/nirmata/ottoflow` (`.github/workflows/release.yaml`, `release-chart` job)
   - In a separate `release-cli` job, runs **GoReleaser** to build the CLI binaries, publish a **GitHub Release**, and update the Homebrew tap (`.github/workflows/release.yaml`, `.goreleaser.yaml`)

## Steps to create a release

### 1. Decide the version

Use [semantic versioning](https://semver.org/): `vMAJOR.MINOR.PATCH` (e.g. `v0.2.0`). For pre-releases: `v0.2.0-alpha1`, `v0.2.0-rc1`.

### 2. Ensure main is ready

- All changes for the release are merged to `main`.
- CI is green on `main`.

### 3. Create and push the tag

From a clean checkout of `main`:

```bash
# Create the tag (annotated tags are recommended for releases)
git tag -a vX.Y.Z -m "Release vX.Y.Z"

# Push the tag to trigger the release workflow
git push origin vX.Y.Z
```

Replace `vX.Y.Z` with your version (e.g. `v0.2.0`).

**Tag format rules** (enforced by the workflow):

- **Stable:** `vX.Y.Z` (e.g. `v0.1.0`, `v1.0.0`)
- **Pre-release:** `vX.Y.Z-prerelease` (e.g. `v0.1.0-alpha2`, `v0.2.0-rc1`)

Invalid tags (e.g. `v0.1` without patch) will cause the workflow to fail.

### 4. Verify the release

- **GitHub Actions:** Open the [Actions](https://github.com/nirmata/ottoflow/actions) tab and confirm the "Release" workflow run for your tag succeeds.
- **GitHub Release:** In the repo **Releases** page, a new release should appear for the tag with the CLI binary archives (`.tar.gz`) and `checksums.txt`, produced by GoReleaser (`.goreleaser.yaml`). The Helm chart is no longer attached to the GitHub Release; it is distributed via GHCR (OCI) only.
- **OCI chart:** Install from OCI to confirm the chart is available:
  ```bash
  helm install ottoflow oci://ghcr.io/nirmata/ottoflow --version X.Y.Z
  ```

### 5. (Optional) Publish release notes

Edit the generated GitHub Release to add release notes (what changed, upgrade notes, etc.).

---

## First-time setup (one-time)

### OCI (GHCR)

No extra setup is required for OCI. The workflow uses `GITHUB_TOKEN` to push to `ghcr.io/nirmata`.

---

## Local development and testing

| Command | Purpose |
|--------|---------|
| `make helm-package` | Package the chart locally (e.g. to `_output/charts/`) |
| `make helm-repo-index` | Generate `index.yaml` for local chart repo testing |

---

## Workflows

| Workflow | Trigger | Purpose |
|----------|---------|---------|
| `ci.yaml` | PR and push to `main` | Build, lint, unit tests, images, Helm lint |
| `release.yaml` | Tag push (`v*.*.*`) | Set chart version from tag, package chart, push to GHCR (OCI); build CLI binaries and publish a GitHub Release via GoReleaser |
