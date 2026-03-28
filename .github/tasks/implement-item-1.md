# implement-item-1

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
None yet.

## 4. Step-by-step plan
1.  [x] Add top-level config pre-processing in `main.go` to handle `:env`, `:log`, `:exec_pre`, `:exec_post` before processing link definitions.
2.  [x] Implement recursive resolution and processing of `:env`.
3.  [x] Setup tee-logging for standard logs based on `:log`.
4.  [x] Implement generic command execution utility for capturing output to logs.
5.  [x] Use command execution utility to run `:exec_pre` before directory processing.
6.  [x] Use command execution utility in `aclWorker` for `icacls`, capturing output correctly.
7.  [x] Use command execution utility to run `:exec_post` after directory processing.

## 5. Summary
Implemented recursive `:env` processing updating custom and OS env. Added multiwriter `:log` configuration. Handled execution of `:exec_pre` and `:exec_post` using `cmd /c`. All subprocess execution, including `icacls`, now tees standard output/error to standard log interface. Code modifications committed via Jujutsu.
