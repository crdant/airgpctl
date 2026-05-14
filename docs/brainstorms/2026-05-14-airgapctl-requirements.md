---
date: 2026-05-14
topic: airgapctl
---

# airgapctl

## Summary

airgapctl is a Go CLI tool that takes a local Replicated `.airgap` bundle, pushes all container images to a self-hosted private registry, and generates a standard-named values file for Helm installs — replacing the manual docker save/load/retag/push + hand-edit Helm values workflow.

---

## Problem Frame

SREs and platform engineers deploying into air-gapped environments receive Replicated `.airgap` bundles via the Replicated download portal. To get a bundle usable, they must: extract the bundle, `docker load` all images, manually retag each one to their private registry path, `docker push` them all, then hand-edit Helm chart values to replace image registry references. This workflow is tedious (dozens of images per bundle), error-prone (typos in retagging, missed images), and scales poorly (repeated per release, per environment). No existing CLI tool automates this consumer side of the airgap bundle pipeline.

---

## Actors

- A1. **SRE / Platform Engineer**: The person responsible for getting an `.airgap` bundle into a private registry and producing the values file that a Helm install consumes.
- A2. **airgapctl**: The CLI tool, invoked by A1.

---

## Key Flows

- F1. **Load bundle into registry + generate values** (primary flow)
  - **Trigger:** A1 runs `airgapctl` with a `.airgap` file, destination registry URL, and credentials.
  - **Actors:** A1, A2
  - **Steps:** A1 provides the `.airgap` file path and destination registry parameters → A2 extracts and parses `airgap.yaml` to discover the image list → A2 pushes each image to the destination registry, reporting progress and per-image success/failure → A2 generates a values file with a configurable path template, using the default or flag-specified output filename → A1 uses the values file with `helm install -f`.
  - **Outcome:** All images are in the destination registry; a values file exists that Helm can consume.
  - ****Covered by:** R1, R2, R3, R4, R5, R6, R7, R8, R11, R12, R13

- F2. **Generate values only** (secondary flow)
  - **Trigger:** A1 runs `airgapctl` in values-only mode, providing the `.airgap` file and destination registry URL.
  - **Actors:** A1, A2
  - **Steps:** A1 provides the `.airgap` file path, destination registry URL, and a flag to skip the push → A2 extracts and parses `airgap.yaml` for the image list → A2 generates the values file without pushing images.
  - **Outcome:** A values file exists; registry push is not performed.
  - **Covered by:** R4, R5, R6, R7, R8

---

## Requirements

**Image loading**
- R1. Push all container images from a local `.airgap` bundle to a destination OCI-compatible registry.
- R2. Accept configurable destination registry parameters: registry URL, credentials (username/password or token), and TLS settings (skip verify, custom CA).
- R3. Report progress during image push — at minimum, images completed vs. total, with elapsed time.
- R11. Handle multi-architecture images correctly — when a manifest list is present, push the list and all referenced platform-specific manifests so every architecture is pullable from the destination registry. (The vandoor builder already produces multi-arch bundles via `copy.CopyAllImages` when images are referenced by digest.)
- R12. Report which images succeeded and which failed on any non-catastrophic error during a push operation.
- R13. Support idempotent push — re-running against the same bundle and destination registry must be safe, with already-present images skipped without error.

**Bundle handling**
- R4. Accept a local `.airgap` file path as the primary input.
- R5. Parse `airgap.yaml` from within the bundle to discover the image list (the `SavedImages` field) and metadata.

**Values file generation**
- R6. Generate a YAML values file that maps each original image reference to its corresponding destination registry path. The values file shape is driven by the target Helm chart's values schema — only images from the bundle that are referenced by that chart receive entries (partial matching). Running against the same bundle with a different chart produces values for that chart's images only.
- R7. Use a standard default output filename (e.g., `images.yaml`) with a CLI flag to override.
- R8. Allow the destination image path to be user-configurable — at minimum, control over the registry host, namespace/prefix, and whether the tag is preserved or overridden.

**Distribution and format**
- R9. Distribute as a single static Go binary with no external runtime dependencies.
- R10. Support the `docker` bundle format (Docker distribution v2 registry-on-disk layout) as produced by the Replicated airgap builder.

---

## Acceptance Examples

- AE1. **Covers R1, R2.** Given a valid `.airgap` bundle and destination registry credentials, when `airgapctl` loads the bundle, all images listed in `airgap.yaml`'s `SavedImages` are present and pullable from the destination registry at their mapped paths.
- AE2. **Covers R4, R5, R6, R8.** Given a valid `.airgap` bundle and a destination registry URL, when `airgapctl` generates the values file, the output YAML contains an entry for every image in `airgap.yaml`'s `SavedImages`, each mapped to the destination registry under the user-specified path template.
- AE3. **Covers R3.** When pushing N images, `airgapctl` outputs progress (e.g., `[3/12]`) with elapsed time visible, and a summary line on completion.
- AE4. **Covers R7, R8.** Given a `.airgap` bundle, destination registry, and a user-specified destination namespace and flag-specified output filename, when `airgapctl` generates the values file, the result exists at the specified path and each image entry reflects the configured namespace/tag.
- AE5. **Covers R12, R13.** Given a `.airgap` bundle and destination registry where 3 of 10 images already exist, when `airgapctl` pushes the bundle, the 3 existing images are skipped, the remaining 7 are pushed, and the final report lists success/failure per image.
- AE6. **Covers R11.** Given a `.airgap` bundle containing a multi-arch image with both `linux/amd64` and `linux/arm64` manifests, when `airgapctl` pushes the bundle, both platform-specific images are pullable from the destination registry.

---

## Success Criteria

- An SRE can take a downloaded `.airgap` file and have all images in their private registry and a usable values file on disk from a single command.
- The generated values file can be passed directly to `helm install -f` without manual editing of image references.
- The tool works without requiring any runtime dependency to be pre-installed in the air-gapped environment — the static binary is self-sufficient.

---

## Scope Boundaries

- Downloading `.airgap` bundles from S3 or OCI registry (**deferred to a subsequent release**).
- Authenticating with the Replicated API (license ID, customer password, JWT sessions) (**deferred to a subsequent release**).
- Embedded cluster bundle support.
- Diff/incremental bundle support.
- Modifying Helm charts or `app.tar.gz` in place.
- Legacy `docker-archive` bundle format.
- Uploading bundles or images to S3 or any remote storage.

---

## Key Decisions

- **Pure Go, no external registry binary**: The tool pushes images directly from the Docker distribution v2 registry-on-disk directory, rather than running a temporary `registry serve` process. This avoids requiring the `registry` binary as a runtime dependency — critical for air-gapped environments where dependencies cannot be fetched.
- **Download deferred, not excluded**: Fetching `.airgap` bundles from the Replicated platform (S3 presigned URLs, OCI registry, API auth) is intentionally deferred to a subsequent release. The MVP assumes the SRE has already downloaded the bundle.

---

## Dependencies / Assumptions

- The `.airgap` bundle uses the `docker` format (Docker distribution v2 registry-on-disk layout in `images/`), which is the primary format produced by the Replicated airgap builder.
- The SRE has already obtained the `.airgap` file locally via the Replicated download portal or other means.
- The destination registry speaks the OCI distribution spec and is network-reachable from the machine running airgapctl.
- The vandoor (Replicated) codebase at `github.com/replicatedhq/vandoor/airgap-builder/` serves as the reference implementation for bundle format and image handling patterns.

---

## Outstanding Questions

### Deferred to Planning

- [Affects R1][Technical] Which Go library to use for reading the Docker distribution v2 directory and pushing to remote registries (go-containerregistry, oras-go, containers/image).
- [Affects R5][Technical] `airgap.yaml`'s `SavedImages` field contains raw image names (not always fully qualified with registry domain). The tool must discover the actual repository path within the Docker distribution v2 directory to resolve images to their on-disk manifests.
- [Affects R8][Technical] What template syntax to use for mapping source image references to destination paths (e.g., `{registry}/{namespace}/{name}:{tag}`).
