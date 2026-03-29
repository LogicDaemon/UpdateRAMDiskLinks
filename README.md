# UpdateRAMDiskLinks

UpdateRAMDiskLinks is a utility designed to redirect caches, logs, and other frequently written folders to a RAM drive. It does not move existing files; instead, it keeps existing folders where they are (renaming them as backups) and creates links in their original locations so that new data is written to the RAM drive.

## Configuration & Path Resolution

Paths inside your `ramdisk-config.yaml` support environment variable expansion (e.g. `%APPDATA%`, `%LOCALAPPDATA%`) and are processed with the following relative path resolution rules:

### 1. Source Keys (Root and Subkeys)

**Root Keys:** Any non-absolute paths configured at the root level of the YAML config describe the *source* files or folders you want to redirect. If a root key is relative, it resolves against the directory where your configuration file is located (`configDir`).
*(A warning is logged if you provide a relative root key, as typically you'll use `%APPDATA%` or absolute system paths).*

**Subkeys:** Nested paths resolve against their parent key's resolved source path. 

### 2. Override Targets (`>`)

The `>` operator lets you override the destination target. 
- If the value is an absolute path, it acts as the exact path for the junction/symlink target.
- If the value is a **relative path**, it automatically resolves against the **expected RAM target's parent directory**.

**Example:**
```yaml
"%LOCALAPPDATA%":
  "opencode":
    ">": "my_custom_cache"
```
Here, the full source path is `%LOCALAPPDATA%\opencode` (e.g., `C:\Users\John\AppData\Local\opencode`). 
The strictly equivalent target on the RAM Drive *would* have been `R:\Users\John\AppData\Local\opencode`.
Because `my_custom_cache` is a relative override, it uses the target's parent directory (`R:\Users\John\AppData\Local`) and resolves the final target structure as `R:\Users\John\AppData\Local\my_custom_cache`.

### 3. Emplaced Directories (`:mkdir`)

The `:mkdir` directive instructs the utility to purely create empty directories.
- If a root level `:mkdir` path is relative, it resolves directly against the root of the **RAM Drive**.
- If a nested `:mkdir` path is relative, it resolves against the parent source path mirrored onto the RAM target. For example, under `%LOCALAPPDATA%\Steam`, `"logs"` becomes `%RAMDrive%\Users\...\AppData\Local\Steam\logs` rather than `%LOCALAPPDATA%\Steam\logs`.
- Additional nested entries inside a `:mkdir` tree continue from the RAM-side path that was just created.

### 4. File Inclusions (`<file`)

The `<` prefix reads lines from a text file and dynamically injects them as keys. 
- If the provided filename is relative, it resolves against the directory where the configuration file was loaded from (`configDir`), completely ignoring the source path currently in context.
- Included paths still follow normal glob semantics: if the resulting full path contains a wildcard, it is only processed when that full path already exists.

## Root Directives

These special keys must be defined at the root level of your YAML configuration.

- **`:env`:** Define dynamic environment variables to use in path resolutions. Supports recursive expansion. If a key starts with `?` (e.g. `"?APPDATA"`), the variable is only set if it's currently undefined or empty in the OS environment.
- **`:log`:** Directs standard outputs (from the utility and executed subprocesses) along with standard error to a specified file. If the path is relative, it resolves against the configuration directory (`configDir`). The absolute evaluated path of the log will be exported to the `LOG` environment variable for spawned tasks. If `:log` is not provided in the YAML configure, the `LOG` environment variable is used as the file path if present. Alternatively, output will strictly default to `stderr` only.
- **`:exec_pre` / `:exec_post`:** Arrays of commands to run *before* or *after* the directory processing phase. Commands are parsed with Windows command-line rules and started directly, without wrapping them in `cmd.exe /c`. All standard output and errors are captured to the log, and the exit code is logged as well. Environment variables specified or expanded in `:env` are applied prior to execution. If you specifically need shell syntax, invoke the shell explicitly in the config.
- **`:uselinkstarget`:** Boolean top-level switch. When enabled, any source that is already a junction/symlink keeps its current destination instead of being repointed to the RAM-disk-derived target. If that preserved destination is missing, the utility recreates it before leaving the existing link in place. Empty value is treated as enabled, so both `":uselinkstarget": true` and a bare `":uselinkstarget":` work.

## Semantics Glossary

- **Empty value:** If the source exists and isn't already a link, the tool will construct the RAM disk target, rename the source (appending a suffix with a date-time stamp), and create a junction/symlink to the RAM structure. If the source is already a junction/symlink that points to the intended target, the tool leaves that link in place and skips recreating it.
- **Existing links with `:uselinkstarget`:** When that root directive is enabled, an already-linked source keeps pointing where it already points, even if that destination is outside the RAM drive. The tool still creates the preserved destination when it is missing.
- **`?` Prefix:** Checks for existence. The app will skip logic for the current key if the underlying localized source doesn't exist on disk, perfect for keeping robust multi-machine lists without unused redundant directories and links.
- **`* and ?` Globs:** Strings containing wildcards are appropriately expanded into active paths at runtime. Except you can't start with a key with "?", as it is reserved for the existence check. But you can put "?" after "\\" to a parent key to use it as a first-character wildcard:
```yaml
"%APPDATA%":
  "?cache": # will check if the exact directory "cache" exists, and skip if it doesn't
  ".\\?cache": # will actually search for directories matching "?cache" pattern (any 1 first char, followed by "cache") and process them
```
- Globs use Go `filepath.Glob` / `filepath.Match` rules, not `cmd.exe` wildcard rules. In particular, `*.*` only matches names that contain a literal dot, so it will **not** match directories like `selectivesyncview` or `dbid%3A...`; use `*` if you mean “any child name”.
- Globs only process existing full-path matches. If you want to create the same child path under every matched parent, put the wildcard on the parent key and nest the child path beneath it, for example:
```yaml
"Storage\\ext\\*":
  "def\\Application Cache":
```
- At each YAML mapping level, non-glob keys are claimed first and glob keys run afterward. A glob therefore skips any exact sibling path already claimed at that same level, even if the glob appears earlier in the file.
- **`":skip"` value:** Reserves a path and performs no link or mkdir action for that key. This is mainly useful for carving an exact path out of a same-level glob, for example:
```yaml
"LocalState":
  "*":
  "EBWebView": ":skip"
```
- **`:defs`:** Purely a repository block for YAML anchors (`&my_cache`). The application silently skips processing this.
- **Unknown `:` directives:** Keys beginning with `:` are treated as directives. Unknown directives are ignored and logged with a warning instead of being interpreted as filesystem paths.

See [ramdisk-config.yaml](ramdisk-config.yaml) for example.
