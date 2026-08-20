# Getting Started

OttoFlow is a Kubernetes-native workflow engine. Workflows are defined as
`Workflow` custom resources and executed either **locally** (in-process, no
cluster required) or **in-cluster** (the controller runs each workflow in a
Kubernetes Job). This page covers building the tooling and running your first
workflow.

Sources for this page: [`README.md`](../README.md),
[`DEVELOPER.md`](../DEVELOPER.md), [`Makefile`](../Makefile),
[`cli/README.md`](../cli/README.md), [`.github/workflows/ci.yaml`](../.github/workflows/ci.yaml).

## Prerequisites

- **Go** — the module targets `go 1.26.0` (`go.mod`). CI resolves its Go
  version from `go.mod` (`.github/workflows/ci.yaml`).
- **Kubernetes cluster 1.20+** — only needed for in-cluster execution
  (`README.md`, `DEVELOPER.md`). Local execution needs no cluster.
- **`kubectl`**, **`make`**, and (for the chart) **Helm v3** — Helm v3.13.0 is
  the version pinned in CI and in the Makefile's tooling (`.github/workflows/ci.yaml`,
  `Makefile` `HELM_VERSION`).

> **Build from source.** `go build ./...` and `go mod download` resolve all
> dependencies from public module sources — no Nirmata org access or special
> credentials required (`CONTRIBUTING.md`, `DEVELOPER.md` → "Status &
> Roadmap"). The `nirmata` model provider requires the enterprise plugin; all
> other providers work out of the box (`README.md`). Container images are
> published at `ghcr.io/nirmata/ottoflow/*`.

## Build

The Makefile drives all builds (`Makefile`).

### CLI

```sh
make build-cli          # builds bin/ottoflow (runs manifests, generate, fmt, vet first)
make build-cli-all      # cross-compile linux/darwin/windows into bin/
make install-cli        # go install into $GOPATH/bin (or $GOBIN)
make install-cli-local  # copy bin/ottoflow to /usr/local/bin (uses sudo)
```

The binary is versioned via ldflags (`git describe`) — check it with
`make cli-version` (`Makefile`).

### Controller binary

```sh
make build              # builds bin/manager from cmd/controller/main.go
```

### Container images

Images are built with [`ko`](https://ko.build/) — there is no Dockerfile
(`Makefile`, `.ko.yaml`). Three images are produced: `controller`,
`agent-executor`, and `workflow-runner`.

```sh
make ko-build           # build all three images locally (no push)
make ko-push            # build and push all three (requires KO_DOCKER_REPO + registry login)
make image-version      # show the tag ko will use
```

The image tag is the git tag when `HEAD` is on one, otherwise
`0.0.0-g<short-sha>`; override with `IMAGE_TAG=` (`Makefile`).

## Run your first workflow (local, no cluster)

Local execution loads workflows from a directory and runs them in-process. This
is the fastest way to try OttoFlow (`README.md`, `cli/README.md`):

```sh
./bin/ottoflow run cluster-overview --workflow-dir samples/workflows
```

Pass inputs with `--input key=value` (repeatable):

```sh
./bin/ottoflow run stale-image-check --workflow-dir samples \
  --input thresholdDays=60 --input namespace=kube-system
```

(`stale-image-check` uses a `stepTemplateRef`, and its StepTemplate lives in
`samples/steptemplates/` — passing the parent `samples` directory loads both.)

Both `samples/workflows/production/cluster-overview.yaml` and
`samples/workflows/production/stale-image-check.yaml` exist in the repo
(`samples/workflows/` contains 90+ example workflows, organized into
`production/`, `features/`, and `testing/` — see `samples/workflows/README.md`).

### Validate before running

`ottoflow validate` runs static checks (DAG cycle detection, `dependsOn`
alignment, undefined `inputs.*`, CEL syntax) without executing steps
(`cli/cmd/validate.go`):

```sh
./bin/ottoflow validate --workflow-dir samples/workflows            # all workflows in a dir
./bin/ottoflow validate cluster-overview --workflow-dir samples/workflows
./bin/ottoflow validate -f samples/workflows/production/cluster-overview.yaml
```

## Run in a cluster

### 1. Install the controller

**Helm (recommended for a real cluster)** — the chart is published to GHCR
as an OCI artifact (`README.md`):

```sh
helm install ottoflow oci://ghcr.io/nirmata/ottoflow --version <version> -n ottoflow --create-namespace
```

**From source (local dev against your kubeconfig cluster)** (`Makefile`, `README.md`):

```sh
make install    # install CRDs (kustomize build config/crd | kubectl apply)
make run        # creates the ottoflow namespace and runs the controller from your host
```

### 2. Apply a sample Workflow

```sh
kubectl apply -f samples/workflows/production/cluster-overview.yaml
```

### 3. Create a WorkflowRun

Without `--workflow-dir`, the CLI creates a `WorkflowRun` in the cluster and
(by default) watches it to completion (`cli/cmd/run.go`):

```sh
./bin/ottoflow run cluster-overview
./bin/ottoflow run my-workflow --input name=World --watch=false   # create and exit
./bin/ottoflow status <workflowrun-name> --output json            # check a run later
```

The CLI verifies the referenced `Workflow` exists before creating the
`WorkflowRun`, and polls status every 2s until a terminal phase or the
`--timeout` (default `10m`) is reached (`cli/cmd/run.go`).

## Test

```sh
make test              # go test with envtest (KUBEBUILDER_ASSETS); prints coverage
make lint              # golangci-lint (v2.11.4, pinned in Makefile and CI)
make test-e2e-kind     # create a kind cluster if needed, then run test/e2e
```

`make test` downloads Kubernetes test binaries for v1.29.0 via `setup-envtest`
(`Makefile` `ENVTEST_K8S_VERSION`). Sample workflows in `samples/workflows/`
double as integration tests (`DEVELOPER.md`).

## What CI does

The `CI` workflow (`.github/workflows/ci.yaml`) runs on pushes to `main`,
`v*.*.*` tags, and pull requests. Its jobs:

- **build** — `make build-cli`, then `helm lint` and `helm template` the chart;
  uploads `bin/ottoflow` as an artifact.
- **lint** — `golangci-lint` (v2.11.4) and a check that all Actions are
  SHA-pinned.
- **test** — `make test` (with cached envtest binaries for K8s 1.29.0).
- **images** — on non-tag refs, builds the three container images with `ko`
  and pushes to `ghcr.io/nirmata/ottoflow`.
