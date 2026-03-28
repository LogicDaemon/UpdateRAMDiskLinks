# `%vscodeRemoteWSLDist%`
The batch script calls `_Distributives.find_subpath.cmd` to dynamically find the `%vscodeRemoteWSLDist%` path. The Go script assumes `%vscodeRemoteWSLDist%` is already defined in the environment. You'll need to ensure this is set in the environment before running `main.go`.

# Disabling Windows Indexing on the RAM Disk:

Batch: Runs ATTRIB +I "%RAMDrive%\*.*" /S /D /L to prevent the Windows Search indexer from indexing temporary/cache files.
Go: This step is completely missing.
