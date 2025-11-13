# Troubleshooting Guide

Common issues and solutions for the Performance Profile Creator MCP Server.

## Container Build Issues

### Error: Unable to retrieve auth token (Red Hat Registry)

**Error Message:**
```
Error: creating build container: initializing source docker://registry.redhat.io/ubi9/python-311:latest: 
unable to retrieve auth token: invalid username/password: unauthorized: 
Please login to the Red Hat Registry using your Customer Portal credentials.
```

**Cause:** The original Containerfile used Red Hat UBI9 base image which requires customer portal credentials.

**Solution:** We've switched to a public Python base image. Use the current `Containerfile`:

```bash
podman build -t performance-profile-creator-mcp:latest .
```

See `BUILD_NOTES.md` for more details on the base image options.

---

## Container Runtime Issues

### Error: SELinux relabeling of /tmp is not allowed

**Error Message:**
```
Error: SELinux relabeling of /tmp is not allowed
```

**Cause:** The test script tried to mount `/tmp` with SELinux relabeling (`:z` flag), but SELinux doesn't allow relabeling the entire `/tmp` directory for security reasons.

**Solution:** Use the simplified test script that doesn't require volume mounts:

```bash
./test_simple.sh performance-profile-creator-mcp:latest
```

This script tests all MCP server functionality without mounting any volumes, avoiding SELinux issues entirely.

**Alternative:** If you need to test with must-gather data, provide a specific subdirectory:

```bash
# Create a test directory
mkdir -p /tmp/my-must-gather
# Use it with the test script
./test_mcp_protocol.sh performance-profile-creator-mcp:latest /tmp/my-must-gather
```

---

## MCP Protocol Issues

### Error: Invalid JSON / trailing characters

**Error Message:**
```
ERROR Received exception from stream: 1 validation error for JSONRPCMessage
      Invalid JSON: trailing characters at line 1 column 12
```

**Cause:** The MCP protocol requires each JSON-RPC message to be on a single line (newline-delimited JSON), but pretty-printed JSON was being sent.

**Solution:** The test scripts now automatically compact JSON to single lines using `jq -c`.

If you're writing your own test scripts, ensure JSON is on a single line:

```bash
# Good - single line
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | podman run ...

# Bad - multi-line
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list"
}' | podman run ...
```

---

## Python/FastMCP Issues

### ImportError: No module named 'fastmcp'

**Cause:** Dependencies not installed.

**Solution:**

For local development:
```bash
pip install -r requirements.txt
```

For container builds:
```bash
# Container includes all dependencies
podman build -t performance-profile-creator-mcp:latest .
```

---

## Must-Gather Parsing Issues

### Error: No must-gather data found

**Cause:** The `must-gather` path doesn't exist or doesn't contain expected files.

**Expected Structure:**
```
must-gather/
├── quay-io-openshift-*/
│   └── nodes/
│       ├── <node-name>/
│       │   ├── sys/devices/system/cpu/
│       │   ├── sys/devices/system/node/
│       │   └── proc/cpuinfo
```

**Solution:**

1. Verify must-gather was collected correctly:
```bash
oc adm must-gather --dest-dir=/path/to/must-gather
```

2. Check the structure:
```bash
ls -R /path/to/must-gather | head -50
```

3. For testing without real must-gather, use the mock data in test scripts:
```bash
./test_simple.sh  # Uses mock data
```

---

## Performance Profile Generation Issues

### PPC Command Fails: podman not found

**Cause:** The simplified `Containerfile` doesn't include `podman` to avoid complex dependencies.

**Solution:**

The `create_performance_profile` tool generates the PPC command but doesn't execute it inside the container. You have two options:

**Option 1: Copy and run the command manually (Recommended)**

```bash
# 1. Generate the command using MCP server
# 2. Copy the generated command
# 3. Run it on your host with podman

podman run --rm -v /path/to/must-gather:/must-gather:z \
  quay.io/openshift-kni/performance-addon-operator-must-gather:4.X-snapshot \
  performance-profile-creator \
  --mcp-name worker-cnf \
  --reserved-cpu-count 4 \
  --rt-kernel true \
  --must-gather-dir-path /must-gather \
  > performance-profile.yaml
```

**Option 2: Use Containerfile with podman (Advanced)**

If you need in-container execution, you can create a custom Containerfile that includes podman. See `BUILD_NOTES.md` for guidance.

---

## Common Usage Questions

### Q: Can I use this without Cursor?

**A:** Yes! The MCP server works with any MCP-compatible client. You can also:
- Use the test scripts directly
- Call the Python modules from your own scripts
- Use `test_local.py` for direct Python API usage

### Q: Do I need must-gather data to test?

**A:** No! The test scripts (`test_simple.sh`, `test_local.py`) work without must-gather data using mock data or predefined templates.

### Q: Can I customize the workload templates?

**A:** Yes! Edit `workload_templates.py` to add your own templates or modify existing ones.

### Q: Does this work on ARM64/aarch64?

**A:** Yes! The Python base image supports multiple architectures. The MCP server itself is architecture-agnostic, but ensure your must-gather data matches your target architecture.

---

## Getting Help

If you encounter issues not covered here:

1. Check the main documentation:
   - `README.md` - Overview and features
   - `QUICKSTART.md` - Quick start guide
   - `BUILD_NOTES.md` - Container build options
   - `TESTING.md` - Testing guide

2. Verify your setup:
```bash
# Check podman version
podman --version

# Check Python version
python3 --version

# Test without container
python3 test_local.py
```

3. Run diagnostic tests:
```bash
# Test 1: Basic MCP protocol
./test_simple.sh

# Test 2: Local Python modules
python3 test_local.py

# Test 3: With must-gather (if available)
./test-server.sh /path/to/must-gather
```

4. Check logs:
```bash
# Run container with verbose output
podman run -it --rm -e MCP_TRANSPORT=stdio \
  performance-profile-creator-mcp:latest
```



