# fix Dropbox Partitions glob pattern

## 1. End goal
Make Dropbox partition directories under `%APPDATA%\\Dropbox\\Partitions` match the config so their nested Electron cache entries are processed and stale RAM-drive links under them are recreated.

## 2. Starting state, constraints
- `main.go` expands wildcard keys with Go `filepath.Glob`, not `cmd.exe` wildcard semantics.
- The config currently uses `"?Dropbox\\Partitions\\*.*": *electron_cache`.
- Dropbox partition directory names such as `selectivesyncview` and `dbid%3Aaadyff3fjbq1bwzgfqswxopxibejulx7nsk` do not contain a dot, so `*.*` matches nothing.
- When a glob has no matches, the current implementation silently skips it, so the log contains no `Partitions` entries.
- `makeLink()` already has coverage proving that, when a matched source is an existing broken link and `:uselinkstarget` is enabled, the missing preserved target is recreated.
- The workspace has a `.jj` directory, so the working description must be updated after code edits.

## 3. Failed attempts
- None yet.

## 4. Step-by-step plan
1. [x] Update the Dropbox `Partitions` glob from `*.*` to `*` so existing partition directories are matched.
2. [x] Add a brief README note clarifying that Go globbing treats `*.*` literally and therefore it only matches names containing a dot.
3. [ ] Run verification (`go test ./...`) and record the result.
