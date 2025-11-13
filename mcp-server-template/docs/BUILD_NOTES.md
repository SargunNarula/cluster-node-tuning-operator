# Container Build Options

> **✅ Current Status:** Container builds successfully with public `python:3.11-slim` image.
> No authentication required!

## Issue with Original Containerfile

The original `Containerfile` used Red Hat UBI9 base image which requires Red Hat Customer Portal credentials:
```dockerfile
FROM registry.redhat.io/ubi9/python-311
```

## Solutions

### ✅ Option 1: Simple Build (Recommended)

**Current Containerfile** - Uses public Python image, no authentication needed:

```bash
podman build -t performance-profile-creator-mcp:latest .
./test_simple.sh  # Test without SELinux issues
```

**What works:**
- ✅ All analysis tools
- ✅ Hardware validation
- ✅ PPC command generation
- ✅ Workload templates
- ✅ Natural language recommendations
- ❌ create_performance_profile tool (can't execute PPC commands)

**Why this is fine:**
The MCP server's main value is **generating validated PPC commands** with explanations. You can copy the generated command and run it manually:

```bash
# MCP server generates this command:
podman run --entrypoint performance-profile-creator ...

# You copy and run it yourself on a machine with PPC access
```

### Option 2: With Red Hat Credentials

If you get Red Hat credentials later:

1. **Login to registry:**
```bash
podman login registry.redhat.io
# Username: your-redhat-username
# Password: your-redhat-password
```

2. **Use original Containerfile:**
```bash
# Backup current
mv Containerfile Containerfile.public

# Restore original (if you saved it)
# Or manually edit to use: FROM registry.redhat.io/ubi9/python-311

podman build -t performance-profile-creator-mcp:latest .
```

### Option 3: Alternative Public Images

Other base images you can use:

**Fedora (similar to RHEL):**
```dockerfile
FROM fedora:39
RUN dnf install -y python3.11 python3-pip podman && dnf clean all
WORKDIR /app
# ... rest of Containerfile
```

**Ubuntu:**
```dockerfile
FROM ubuntu:22.04
RUN apt-get update && \
    apt-get install -y python3.11 python3-pip podman && \
    apt-get clean
WORKDIR /app
# ... rest of Containerfile
```

**Alpine (smallest):**
```dockerfile
FROM python:3.11-alpine
RUN apk add --no-cache podman
WORKDIR /app
# ... rest of Containerfile
```

## Current Build Status

✅ **Modified to use `python:3.11-slim`** - No authentication required

## Quick Test

After building:

```bash
# Verify image built
podman images | grep performance-profile-creator-mcp

# Quick test
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  podman run -i --rm \
    -e MCP_TRANSPORT=stdio \
    performance-profile-creator-mcp:latest | jq .
```

Should return JSON with 6 tools.

## Trade-offs

| Base Image | Pros | Cons |
|------------|------|------|
| **python:3.11-slim** (current) | ✅ No auth<br>✅ Small size<br>✅ Fast build | ❌ No podman (can't exec PPC)<br>⚠️ Debian-based |
| **registry.redhat.io/ubi9/python-311** | ✅ Official RHEL<br>✅ Podman included<br>✅ Full functionality | ❌ Requires Red Hat credentials<br>❌ Larger size |
| **fedora:39** | ✅ No auth<br>✅ Podman available<br>✅ RPM-based | ⚠️ Larger size<br>⚠️ More dependencies |

## Recommendation

**For most users: Use current `python:3.11-slim` Containerfile**

Why?
- The MCP server's core value is intelligent command generation, not execution
- All analysis, validation, and generation tools work perfectly
- You can execute generated commands manually or via CI/CD
- No authentication hassles

**Only switch to UBI9 if:**
- You have Red Hat credentials
- You specifically need the `create_performance_profile` tool to execute PPC commands in-container
- You're deploying in Red Hat environment and want consistency

## Next Steps

1. **Build with current Containerfile:**
```bash
podman build -t performance-profile-creator-mcp:latest .
```

2. **Test it:**
```bash
./test_mcp_protocol.sh
```

3. **Use it:**
- Configure in Cursor
- Generate validated PPC commands
- Execute commands separately where PPC is available

You don't lose any intelligence or validation - just the in-container command execution!

