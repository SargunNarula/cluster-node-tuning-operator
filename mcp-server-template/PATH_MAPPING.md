# Path Mapping for Container Environments

## Overview

When running the Performance Profile Creator MCP server in a container, paths from the host system need to be mapped to their corresponding paths inside the container. The server now includes **automatic path resolution** to handle this transparently.

## Problem

When you mount a directory like this:
```bash
-v /home/user/must-gather:/must-gather:z
```

The host path `/home/user/must-gather` is available inside the container as `/must-gather`. But if you pass the host path to the MCP tools, it won't work because the container doesn't have access to `/home/user/must-gather`.

## Solution

The MCP server now automatically resolves paths based on configured mappings. You can use **host paths** or **container paths** interchangeably, and the server will figure it out.

## Configuration Methods

### Method 1: JSON Array (Recommended)

Set the `MCP_PATH_MAPPINGS` environment variable with a JSON array of `[host_path, container_path]` pairs:

```json
{
  "mcpServers": {
    "performance-profile-creator": {
      "command": "podman",
      "args": [
        "run", "-i", "--rm",
        "-v", "/home/user/must-gather:/must-gather:z",
        "-e", "MCP_TRANSPORT",
        "-e", "MCP_PATH_MAPPINGS",
        "performance-profile-creator-mcp:latest"
      ],
      "env": {
        "MCP_TRANSPORT": "stdio",
        "MCP_PATH_MAPPINGS": "[[\"/home/user/must-gather\", \"/must-gather\"]]"
      }
    }
  }
}
```

**Multiple Mappings:**
```json
"MCP_PATH_MAPPINGS": "[[\"/home/user/must-gather\", \"/must-gather\"], [\"/data\", \"/app/data\"]]"
```

### Method 2: Individual Environment Variables

Use numbered environment variables for individual mappings:

```bash
-e MCP_PATH_MAP_0="/home/user/must-gather:/must-gather"
-e MCP_PATH_MAP_1="/data:/app/data"
```

In `mcp.json`:
```json
"env": {
  "MCP_PATH_MAP_0": "/home/user/must-gather:/must-gather",
  "MCP_PATH_MAP_1": "/data:/app/data"
}
```

### Method 3: Auto-Detection

If no explicit mappings are configured, the server automatically detects common mount points like:
- `/must-gather`
- `/data`
- `/workspace`

If these exist in the container, the server will use them as-is.

## Usage Examples

### Example 1: Using Host Path (Auto-Resolved)

```bash
# Your mcp.json has the path mapping configured
# You can now use either format:

# Using host path (will be auto-resolved to /must-gather)
analyze_cluster_hardware("/home/user/must-gather")

# Using container path directly
analyze_cluster_hardware("/must-gather")

# Both work the same!
```

### Example 2: Subdirectories

Path resolution works with subdirectories too:

```bash
# Host path with subdirectory
analyze_cluster_hardware("/home/user/must-gather/subfolder")

# Automatically resolves to:
# /must-gather/subfolder
```

### Example 3: Multiple Mount Points

```json
"MCP_PATH_MAPPINGS": "[
  [\"/home/user/cluster-data/must-gather\", \"/must-gather\"],
  [\"/home/user/output\", \"/output\"]
]"
```

```bash
# Both paths get resolved correctly
analyze_cluster_hardware("/home/user/cluster-data/must-gather")
create_performance_profile(ppc_command, output_path="/home/user/output/profile.yaml")
```

## Debugging Path Resolution

The server logs path resolution operations:

```
[Path Mapping] /home/user/must-gather -> /must-gather
[Path Mapping] Initialized 1 path mapping(s)
[Path Resolution] Resolved: /home/user/must-gather -> /must-gather
[Path Resolution] Path exists as-is: /must-gather
```

Enable verbose output to see path resolution in action:
```bash
podman logs <container-name>
```

## Complete Example Configuration

### mcp.json
```json
{
  "mcpServers": {
    "performance-profile-creator": {
      "command": "podman",
      "args": [
        "run",
        "-i",
        "--rm",
        "-v", "/home/snarula/Documents/work/repos/cluster-node-tuning-operator/must-gather:/must-gather:z",
        "-e", "MCP_TRANSPORT",
        "-e", "MCP_PATH_MAPPINGS",
        "performance-profile-creator-mcp:latest"
      ],
      "env": {
        "MCP_TRANSPORT": "stdio",
        "MCP_PATH_MAPPINGS": "[[\"/home/snarula/Documents/work/repos/cluster-node-tuning-operator/must-gather\", \"/must-gather\"]]"
      }
    }
  }
}
```

### Usage in Code/Chat
```python
# Now you can use the host path naturally!
result = analyze_cluster_hardware(
    "/home/snarula/Documents/work/repos/cluster-node-tuning-operator/must-gather"
)

# The server automatically resolves it to /must-gather inside the container
```

## Benefits

✅ **User-Friendly**: Use familiar host paths without remembering container paths  
✅ **Flexible**: Supports multiple mounting schemes  
✅ **Transparent**: Works seamlessly in both local and containerized environments  
✅ **Robust**: Falls back to original path if no mapping found  
✅ **Debuggable**: Clear logging of path resolution operations  

## Container Detection

The server automatically detects if it's running in a container by checking for:
- `/.dockerenv` (Docker)
- `/run/.containerenv` (Podman)

This enables smart auto-detection behavior.

## Best Practices

1. **Always configure path mappings** in `mcp.json` for predictable behavior
2. **Use absolute paths** for both host and container paths
3. **Match your volume mounts** exactly in the path mappings
4. **Check logs** if paths aren't resolving as expected
5. **Test both host and container paths** to ensure mapping works

## Troubleshooting

### Path not found
```
Error: Must-gather path does not exist: /home/user/must-gather
```
**Solution**: Check that `MCP_PATH_MAPPINGS` is correctly configured

### Path not resolving
**Check**:
1. Is the path mapping configured in `mcp.json`?
2. Does the volume mount match the mapping?
3. Are you using absolute paths?

### View current mappings
Check container logs on startup:
```bash
podman logs <container-name> | grep "Path Mapping"
```

Should show:
```
[Path Mapping] /home/user/must-gather -> /must-gather
[Path Mapping] Initialized 1 path mapping(s)
```

