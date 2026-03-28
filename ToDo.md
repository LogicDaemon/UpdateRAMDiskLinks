# 1. Add settings to avoid necessity for `Move Dirs to RAMdisk.cmd`:

```yaml
":exec_pre":
  - ATTRIB +I "%RAMDrive%\*.*" /S /D /L
":exec_post":
  - COMPACT /C "%LogPath%" /Q
":env":
  # Env vars starting with "?" are only set if they are not yet defined or empty
  # Note that the order of items in the env section is not guaranteed,
  # so the program must resolve them recursively
  # but once resolved, the result should be saved to customEnv
  "?APPDATA": "%USERPROFILE%\\AppData\\Roaming"
  "?LOCALAPPDATA": "%USERPROFILE%\\AppData\\Local"
  "?USERPROFILE": "d:\\Users\\LogicDaemon"
":log": "%RAMDrive%\\RAMDiskLinker.log"  # if relative, base is configDir
```

for `exec_*` to work, os environment variables need to be updated before executing the commands.
The commands stdout/stderr should be tee-ed to the log file, and return code should be logged too.
icacls used to copy ACL should use the same mechanism, so its logs are also captured.

# 2. Add subconfiguration loading

something like:

```yaml
":subconfig": "ramdisk-config.%COMPUTERNAME%.yaml"
":subconfig": "ramdisk-config.%USERNAME%.yaml"
```

# 3. Conflicts and dependencies in the config

Some links may conflict with each other, or may require other links or directories to be created first.
Ex:
```yaml
"%LOCALAPPDATA%\\opencode":
"%USERPROFILE%\\.cache":
  "opencode":
    ">": "%LOCALAPPDATA%\\opencode\\cache"
```

The problem above is that if `"%LOCALAPPDATA%\\opencode\\cache"` created first, when `"%LOCALAPPDATA%\\opencode"` will be renamed it will take away the cache directory.

To resolve that, program must build a dependency graph and then execute the links in the correct order.
