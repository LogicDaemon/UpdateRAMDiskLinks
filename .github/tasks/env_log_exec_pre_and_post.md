# env, log, exec_pre and _post

## 1. End goal
Implement item 1 from ToDo.md to natively support `:env`, `:log`, `:exec_pre`, and `:exec_post` directives in the YAML configuration. 
- `:env`: allow variable assignments and conditionally if prefixed with `?`. Needs recursive resolution.
- `:log`: specify log file. Output of the app and subprocesses (like `exec_*` and `icacls`) needs to be tee-ed to both stdout and this file.
- `:exec_pre` / `:exec_post`: execute shell commands before and after processing directories, and tee their stdout/stderr to the log. Update the OS environment before executing these commands based on customEnv.

## 2. Starting state, constraints
- `main.go` parses the config as `yaml.Node` and walks it.
- Logging currently uses standard `log.Printf`.
- Environment is stored in `customEnv` map and OS environment.
- Subprocesses (`icacls`) are currently run inside `aclWorker` using `exec.Command` and ignoring stdout/stderr.

## 3. Failed attempts
- Used `cmd /c` for `:exec_pre` / `:exec_post`; this produced incorrect path handling for commands such as `ATTRIB +I "R:\*.*" /S /D /L`.
- Used `path.IsAbs()` instead of `filepath.IsAbs()` in Windows path processing, which treated valid Windows absolute paths such as `d:\Users\LogicDaemon\AppData\Local` as relative.
- Processed `:mkdir` as both a directive and a normal path because the directive branch did not `continue`, which caused `:mkdir` to be appended to `configDir`.
- Joined root RAM-drive paths with `filepath.Join(ramDrive, ...)` while `ramDrive` was stored as `R:`, which produced `R:Temp` instead of `R:\Temp`.

## 4. Step-by-step plan
1.  [x] Add top-level config pre-processing in `main.go` to handle `:env`, `:log`, `:exec_pre`, `:exec_post` before processing link definitions.
2.  [x] Implement recursive resolution and processing of `:env`.
3.  [x] Setup tee-logging for standard logs based on `:log`.
4.  [x] Implement generic command execution utility for capturing output to logs.
5.  [x] Use command execution utility to run `:exec_pre` before directory processing.
6.  [x] Use command execution utility in `aclWorker` for `icacls`, capturing output correctly.
7.  [x] Use command execution utility to run `:exec_post` after directory processing.
8.  [x] Wait for queued ACL propagation to finish before running `:exec_post`.

## 5. Current verification
- Replaced `cmd /c` wrapping with direct Windows command-line decomposition and direct process execution.
- Fixed Windows absolute-path detection to use `filepath.IsAbs()`.
- Fixed root RAM-drive joins to normalize `R:` into `R:\` before joining relative paths.
- Fixed directive handling so `:mkdir` is not processed as a filesystem path and unknown `:` directives are logged and skipped.
- Added ACL job tracking so `:exec_post` runs only after queued `icacls` save/restore operations complete.
- Ran `gofmt -w main.go config_exec.go`.
- Ran `go build` successfully.
- Ran `go test ./...` successfully.

