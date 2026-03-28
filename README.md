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
- Nested `:mkdir` paths resolve against their parent `:mkdir` paths on the RAM Drive.

### 4. File Inclusions (`<file`)

The `<` prefix reads lines from a text file and dynamically injects them as keys. 
- If the provided filename is relative, it resolves against the directory where the configuration file was loaded from (`configDir`), completely ignoring the source path currently in context.

## Root Directives

These special keys must be defined at the root level of your YAML configuration.

- **`:env`:** Define dynamic environment variables to use in path resolutions. Supports recursive expansion. If a key starts with `?` (e.g. `"?APPDATA"`), the variable is only set if it's currently undefined or empty in the OS environment.
- **`:log`:** Directs standard outputs (from the utility and executed subprocesses) along with your console to a specified file. If the path is relative, it resolves against the configuration directory (`configDir`).
- **`:exec_pre` / `:exec_post`:** Arrays of shell commands (executed via `cmd.exe /c`) to run *before* or *after* the directory processing phase. All standard output and errors are captured to the log. Environment variables specified or expanded in `:env` are applied prior to execution.

## Semantics Glossary

- **Empty value:** If the source exists and isn't already a link, the tool will construct the RAM disk target, rename the source (appending a suffix with a date-time stamp), and create a junction/symlink to the RAM structure.
- **`?` Prefix:** Checks for existence. The app will skip logic for the current key if the underlying localized source doesn't exist on disk, perfect for keeping robust multi-machine lists without unused redundant directories and links.
- **`* and ?` Globs:** Strings containing wildcards are appropriately expanded into active paths at runtime. Except you can't start with a key with "?", as it is reserved for the existence check. But you can put "?" after "\\" to a parent key to use it as a first-character wildcard:
```yaml
"%APPDATA%":
  "?cache": # will check if the exact directory "cache" exists, and skip if it doesn't
  ".\\?cache": # will actually search for directories matching "?cache" pattern (any 1 first char, followed by "cache") and process them
```
- **`:defs`:** Purely a repository block for YAML anchors (`&my_cache`). The application silently skips processing this.

See [ramdisk-config.yaml](ramdisk-config.yaml) for example.
