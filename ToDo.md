# Add subconfiguration loading

something like:

```yaml
":subconfig": "ramdisk-config.%COMPUTERNAME%.yaml"
":subconfig": "ramdisk-config.%USERNAME%.yaml"
```

# Conflicts and dependencies in the config

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
