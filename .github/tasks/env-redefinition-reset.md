# End goal
Ensure `:env` definitions are resolved from the config-defined dependency graph instead of stale values already present in `customEnv` or the process environment, while restoring untouched variables when a config definition cannot be resolved.

# Starting state, constraints
- `processEnvBlock` currently resolves each definition via `expandEnv`, which consults `customEnv` first and then `os.LookupEnv`.
- When `USERPROFILE`, `APPDATA`, and `LOCALAPPDATA` are all defined in `:env`, stale pre-existing values for `USERPROFILE` can leak into later expansions.
- Requested behavior: before the resolution loop, collect the non-optional variable names defined by the block, remove those names from both `customEnv` and the OS environment, and keep local backups.
- After resolution finishes, restore any variable that still was not defined, and log a warning for each restored variable.
- Keep existing `?VAR` semantics: optional variables should not be cleared or backed up, so pre-defined values remain in effect and only truly missing/empty ones are filled in.

# Failed attempts
- First reset pass also cleared `?VAR` definitions, which made optional variables behave like mandatory ones during the temporary reset. Revised to only clear non-optional keys.

# Step-by-step plan
- [x] Add helpers/tests for temporarily clearing env vars declared in a `:env` block.
- [x] Update `processEnvBlock` to clear backed-up variables before iterative expansion and restore unresolved ones afterward.
- [x] Verify with targeted tests, including the stale `USERPROFILE` regression, unresolved-definition restoration, and preserved `?VAR` values.
