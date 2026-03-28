# use existing links target

## 1. End goal
Add a top-level `:uselinkstarget` configuration directive that preserves the destination of an already existing junction/symlink instead of repointing it to the RAM disk, while still creating that existing destination when it is missing.

## 2. Starting state, constraints
- `main.go` currently treats existing links in `makeLink()` as reusable only when they already point at the newly computed target.
- Existing link targets are already readable through `os.Readlink()` and normalized by `normalizeLinkTargetPath()` / `linkPointsToTarget()`.
- Root directives are parsed inline in `main()`, and `processNode()` must skip known `:` directives so they are not treated as filesystem paths.
- The workspace has a `.jj` directory, so the working description must be updated after code edits.

## 3. Failed attempts
- Created this task file empty first; filling it in immediately.

## 4. Step-by-step plan
1. [x] Parse and recognize a top-level `:uselinkstarget` directive, defaulting to disabled.
2. [x] Reuse the current destination of existing links when the directive is enabled, and ensure that destination exists before returning.
3. [x] Add regression tests for preserving an existing junction target and for directive parsing semantics.
4. [x] Update README / sample config documentation for the new directive.
5. [x] Run formatting, tests, and build verification; record the result.

## 5. Current verification
- Added `useExistingLinksTarget` and `parseDirectiveBool()` in `main.go`, then parsed `:uselinkstarget` from the root YAML mapping before processing the tree.
- Updated `processNode()` so `:uselinkstarget` is treated as a known directive rather than a filesystem path.
- Added `currentLinkTarget()` and updated `makeLink()` so existing junctions/symlinks keep their current destination when the directive is enabled, while still creating that destination if it is missing.
- Added `main_test.go` coverage for:
	- parsing `:uselinkstarget` values (`true`, empty, `off`, invalid mapping),
	- preserving an existing junction target and recreating its missing destination instead of repointing it to the configured RAM target.
- Updated `README.md` and `ramdisk-config.yaml` to document the directive.
- Ran `gofmt -w main.go main_test.go` successfully.
- Ran `go test ./...` successfully.
- Ran `go build ./...` successfully.
