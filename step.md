# EED — Critical Environment and Dependency Resolution Fixes

## Objective

Fix four areas of EED before its first real Arch Linux installation test.

Preserve all existing functionality.

Do not introduce artificial compatibility or suppress warnings to make Test Drive appear successful.

## 1. Manifest Resource Discovery

Current issue:
EED cannot reliably locate components/manifests when launched from different directories.

Implement a resource discovery system with the following priority:

1. Explicit --data-dir CLI argument.
2. EED_DATA_DIR environment variable.
3. Embedded built-in component manifests using Go embed.
4. System resource directory: /usr/share/eed.
5. Development repository root discovery.

Development root discovery may inspect parent directories for valid project markers:
- go.mod
- components/manifests

Do not assume the executable is always located inside dist/.

Do not simply navigate two directories upward from os.Executable().

Resolve executable symlinks when necessary.

Return actionable errors instead of panicking.

Requirements:
- Works when launched from the repository root.
- Works when launched from dist/.
- Works using an absolute executable path.
- Works from unrelated working directories when installed.
- Supports explicitly configured external component directories.

Add tests for all launch locations.

## 2. Arch Shared Library Dependency Resolution

Current issue:
The dependency resolver attempts to query shared library sonames such as libssl.so and libcurl.so as ordinary package names.

Do NOT skip dependencies containing .so.

Do NOT silently ignore virtual dependencies such as opengl-driver.

Implement proper Arch dependency resolution using package metadata and providers.

Requirements:

- Parse ordinary package dependencies.
- Parse version constraints.
- Parse shared-library soname dependencies.
- Inspect package provides metadata.
- Resolve available providers from Arch sync repository metadata.
- Use appropriate file-database queries when filename ownership must be established.
- Distinguish unresolved dependencies from unavailable repository metadata.
- Preserve optional dependency semantics.
- Distinguish runtime dependencies from build dependencies.
- Never invent providers or dependency versions.

Use libalpm or a reliable package metadata parser where appropriate.

Use Pacman read-only queries during Test Drive.

Do not synchronize repositories automatically.

Do not perform package installation.

Add regression tests for:
- libssl.so dependencies.
- libcurl.so dependencies.
- Versioned soname requirements.
- Virtual package providers.
- opengl-driver.
- Missing providers.
- Stale repository metadata.
- Optional dependencies.

## 3. Virtual GPU and Graphics Capability Detection

Current issue:
The hardware validator only accepts Intel, AMD and NVIDIA GPU vendors, causing QEMU virtual GPUs to be rejected.

Improve GPU detection without weakening genuine graphics compatibility requirements.

Recognize documented virtual GPU hardware identifiers, including relevant QEMU/Virtio devices.

PCI vendor identifiers and device identifiers must be treated separately.

Do not mix vendor names and PCI IDs in the same untyped string list.

Separate these properties:

- GPU detected.
- Physical or virtual GPU.
- Kernel driver bound.
- DRM device available.
- Render node available.
- OpenGL renderer.
- Hardware accelerated rendering available.
- Software rendering fallback.

A detected Virtio GPU must not automatically be considered 3D accelerated.

Allow virtual hardware for diagnostics and package planning.

For components requiring graphics acceleration, evaluate the actual rendering capability instead of requiring Intel/AMD/NVIDIA vendor identity.

If acceleration is unavailable, report a specific warning or component compatibility limitation.

Add tests for:
- Intel GPU.
- AMD GPU.
- NVIDIA GPU.
- Virtio GPU.
- QXL.
- Software renderer.
- Missing render node.
- Missing GPU device.
- Hardware acceleration unavailable.

## 4. RAM Requirements and Snapshot Assessment

Current issue:
A hardcoded memory threshold may block otherwise valid installations.

Implement component-aware resource assessment.

Requirements:

- Estimate RAM requirements for selected components.
- Distinguish minimum requirements from recommended requirements.
- Distinguish total RAM from currently available RAM.
- Evaluate the actual selected profile.
- Allow users to inspect resource constraints.
- Never bypass a true minimum requirement silently.
- Report insufficient recommended resources as warnings when appropriate.

Snapshot assessment:

- Detect filesystem type before assessing snapshot compatibility.
- Do not assume Snapper works as a verified protection mechanism simply because its executable exists.
- Distinguish Btrfs, LVM thin snapshots and discontinued/limited filesystem support.
- Inspect existing subvolume and mount layouts.
- Do not create Snapper configurations automatically.
- Do not modify filesystem layouts.
- Do not claim rollback is verified without a real rollback test.

Support an external VM snapshot as an optional testing safeguard, but never report it as an EED-managed filesystem snapshot.

## Testing

- Preserve the Arch backend.
- Preserve Test Drive read-only guarantees.
- Use mocked executors for unit tests.
- Add regression tests for all four areas.

Run:
- gofmt
- go test ./...
- go vet ./...
- go test -race ./...

Do not perform real package installation during tests.

## Deliverables

1. Resource discovery implementation.
2. Correct Arch dependency resolution.
3. Virtual GPU capability detection.
4. Component-aware RAM assessment.
5. Correct snapshot capability assessment.
6. Regression test results.
7. List of modified files.
8. Remaining limitations.

Do not claim installation readiness unless the installation transaction, configuration deployment, service manager, verification and recovery workflows are implemented and tested.