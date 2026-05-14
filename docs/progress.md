# airgapctl Implementation Progress

## Completed
- [x] Initialize Go module and project structure (`go.mod`, `Makefile`, `cmd/airgapctl/main.go`)
- [x] Create package stubs (`bundle`, `registry`, `values`, `config`)
- [x] **Bundle extraction and `airgap.yaml` parsing (R4, R5)**
  - `OpenBundle()` opens a `.airgap` tar archive and parses `airgap.yaml`
  - `ExtractTo()` safely extracts all bundle contents with directory traversal protection
  - Table-driven tests cover happy path, edge cases, error paths, and security
  - Learning documented in `docs/solutions/design-patterns/safe-tar-extraction-yaml-parsing-airgap-bundles.md`
- [x] **Docker distribution v2 directory traversal (R10, R11)**
  - `Walker` traverses `images/` directory and resolves manifest lists vs single-arch manifests
  - `ResolveImages()` maps `SavedImages` raw names to on-disk repository paths via tag links and blob reading
  - Multi-arch detection extracts platform-specific manifest digests from OCI indexes / Docker manifest lists
  - Table-driven tests cover single-arch, multi-arch, multiple repos, missing tags, invalid blobs, and fully-qualified registries
- [x] **Values file generation (R6, R7, R8)**
  - `Generator` produces YAML values files with configurable path template (`{registry}/{namespace}/{name}:{tag}`)
  - `ChartMatcher` implements partial matching for chart-aware filtering
  - Tag override support, duplicate image name detection, and default template with empty-namespace cleanup
  - 13 table-driven tests cover happy path, custom templates, tag override, filtering, chart-aware matching, empty image sets, invalid templates, duplicate detection, and helper functions
  - Learning documented in `docs/solutions/design-patterns/template-based-helm-values-generation-airgap-images.md`

## In Progress
_None — pick from Backlog._

## Backlog (Priority Order)
1. **Registry push logic (R1, R2, R3, R11, R12, R13)**
   - Pure Go push directly from on-disk Docker v2 layout (no `registry serve`)
   - Multi-arch manifest list support
   - Idempotent push (skip already-present images)
   - Progress reporting (`[3/12]` + elapsed time)
   - Per-image success/failure reporting

3. **Registry push logic (R1, R2, R3, R11, R12, R13)**
   - Pure Go push directly from on-disk Docker v2 layout (no `registry serve`)
   - Multi-arch manifest list support
   - Idempotent push (skip already-present images)
   - Progress reporting (`[3/12]` + elapsed time)
   - Per-image success/failure reporting

4. **CLI integration**
   - Wire all packages into `cmd/airgapctl/main.go`
   - Flags: bundle path, registry URL, credentials, output file, namespace, etc.

5. **Acceptance tests**
   - AE1: End-to-end bundle load to registry
   - AE2: Values file generation
   - AE3: Progress reporting
   - AE4: Custom namespace and output filename
   - AE5: Idempotent push with skip reporting
   - AE6: Multi-arch image push

## Deferrals
- Downloading `.airgap` from S3/OCI (deferred to subsequent release)
- Replicated API authentication (deferred to subsequent release)
- Embedded cluster bundle support
- Diff/incremental bundle support
