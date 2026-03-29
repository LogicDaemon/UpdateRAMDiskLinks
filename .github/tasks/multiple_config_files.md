# multiple config files

## 1. End goal
Allow `UpdateRAMDiskLinks` to accept and process more than one YAML config file in a single invocation, in the exact order provided on the command line.
- Earlier config files must be able to define `:env` variables consumed by later config files.
- Relative paths (`:log`, `<file`, root keys) must resolve against the directory of the config file that declared them.
- Existing single-config behavior must remain intact.

## 2. Starting state, constraints
- `main.go` currently accepts exactly one config path from `os.Args[1]`.
- `configDir` is global and is used by relative resolution for root keys, `:log`, and `<file` includes.
- Root directives are preprocessed from the parsed YAML document before `processNode` walks the mappings.
- Processing order matters because `:env` mutates the process environment.

## 3. Failed attempts
- Initial regression test used `:log` and exposed that `setupLog` left the log file handle open, which broke temp-directory cleanup on Windows. Fixed by tracking the active log file and closing the previous handle when switching logs, plus closing it on exit/cleanup.

## 4. Step-by-step plan
1. [x] Refactor config loading/processing into a helper that works for one config path at a time and updates `configDir` for that file.
2. [x] Iterate over all CLI config arguments in order, applying each config fully before moving to the next.
3. [x] Add tests covering ordered multi-config processing and per-config relative path resolution.
4. [x] Update README usage/documentation for multiple config files.
5. [x] Run formatting, build, and tests.

## 5. Current verification
- Added `resolveConfigDir`, `loadConfigDocument`, and `processConfig` to process each CLI config file in order while re-binding `configDir` per file.
- Updated CLI usage to accept `'<config.yaml> [more-config.yaml ...]'`.
- Inlined the top-level multi-config loop directly into `main()` and removed the dedicated `processConfigPaths` helper and its direct regression test.
- Updated README to document multi-config invocation order and per-file relative path resolution.
- Added log-file handle tracking so repeated `:log` setup does not leak file handles across configs.
- Ran `gofmt -w main.go config_exec.go main_test.go`.
- Ran `go test ./...` successfully.
- Ran `go build ./...` successfully.
