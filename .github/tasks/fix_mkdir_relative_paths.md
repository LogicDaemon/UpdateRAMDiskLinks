# fix :mkdir relative path resolution

## 1. End goal
Make `:mkdir` resolve relative paths against the parent source path mirrored onto the RAM target instead of the live source directory. Absolute `:mkdir` paths must keep their current behavior.

## 2. Starting state, constraints
- `processNode` calls `mkDirs(valNode, basePath)` with the current resolved source path.
- `mkDirs` currently joins relative `:mkdir` entries directly under `basePath`, so nested directives under entries like `%LOCALAPPDATA%\Steam` create real directories inside `%LOCALAPPDATA%\Steam`.
- `getRAMTarget` already mirrors an absolute source path onto the RAM target root.
- Root-level relative `:mkdir` entries should still resolve under the RAM target root because there is no parent source path.
- Documentation in `README.md` currently describes the old nested `:mkdir` behavior and must be updated.

## 3. Failed attempts
- None yet.

## 4. Step-by-step plan
1. [ ] Inspect the existing `mkDirs` logic and identify the smallest change that makes relative paths use the mirrored parent target.
2. [ ] Implement the path-resolution change without affecting absolute `:mkdir` entries.
3. [ ] Add regression tests for nested relative `:mkdir` entries and absolute passthrough behavior.
4. [ ] Update `README.md` to describe the new `:mkdir` resolution rules.
5. [ ] Run formatting/tests and record the verification results.
