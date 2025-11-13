# Testing Guide for Performance Profile Creator MCP Server

This guide covers all testing approaches from simple to advanced.

## 🎯 Quick Start - Choose Your Testing Method

| Method | Speed | Realism | Best For |
|--------|-------|---------|----------|
| [Local Python](#method-1-local-python-testing) | ⚡ Fast | Mock data | Development, quick iteration |
| [Container](#method-2-container-based-testing) | 🐢 Medium | Mock/Real | Pre-deployment testing |
| [MCP Protocol](#method-3-mcp-protocol-testing) | 🐢 Medium | Real | Protocol validation |
| [End-to-End](#method-4-end-to-end-workflow) | 🐌 Slow | Real | Full integration testing |
| [Cursor/Client](#method-5-cursor-integration) | 🐌 Slow | Real | User experience testing |

---

## Method 1: Local Python Testing

**Fastest way to test** - no container building, uses mock data.

### Prerequisites
```bash
cd mcp-server-template
pip install -r requirements.txt
```

### Run Tests
```bash
# Test with mock data
./test_local.py

# Test with real must-gather (if available)
./test_local.py /path/to/must-gather
```

### What It Tests
- ✅ Workload templates
- ✅ Must-gather parsing (mock or real)
- ✅ Hardware validation
- ✅ PPC command generation
- ✅ Natural language recommendations

### Expected Output
```
==================================================================
  MCP SERVER LOCAL TESTING SUITE
==================================================================

==================================================================
TEST 1: Workload Templates
==================================================================

✓ Found 7 workload templates:

  • 5g-ran: 5G RAN (Radio Access Network)
    RT Kernel: ✓
    DPDK: ✓
    Power Mode: ultra-low-latency
  ...

==================================================================
  ✅ ALL TESTS PASSED
==================================================================
```

---

## Method 2: Container-Based Testing

**Tests the actual container** that will be deployed.

### Step 1: Build the Container
```bash
cd mcp-server-template
podman build -t performance-profile-creator-mcp:latest .
```

Expected output:
```
STEP 1/7: FROM registry.redhat.io/ubi9/python-311
...
STEP 7/7: CMD ["python", "mcp_server.py"]
COMMIT performance-profile-creator-mcp:latest
Successfully tagged localhost/performance-profile-creator-mcp:latest
```

### Step 2: Verify Image
```bash
podman images | grep performance-profile-creator-mcp
```

### Step 3: Test the Container
```bash
# Quick smoke test
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  podman run -i --rm \
    -e MCP_TRANSPORT=stdio \
    performance-profile-creator-mcp:latest
```

Expected: JSON response listing 6 tools

---

## Method 3: MCP Protocol Testing

**Tests MCP protocol compliance** with all tools.

### Run Protocol Tests
```bash
# Without must-gather (uses mock data)
./test_mcp_protocol.sh

# With must-gather
./test_mcp_protocol.sh performance-profile-creator-mcp:latest /path/to/must-gather
```

### What It Tests
- ✅ tools/list
- ✅ tools/call (all 6 tools)
- ✅ resources/list
- ✅ prompts/list
- ✅ JSON-RPC protocol compliance

### Sample Output
```
🧪 MCP Protocol Testing
=======================

Using image: performance-profile-creator-mcp:latest
Must-gather path: /tmp

✓ Image found

📤 Test: List Available Tools
Request:
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/list"
}

Response:
{
  "jsonrpc": "2.0",
  "id": 1,
  "result": {
    "tools": [
      {
        "name": "analyze_cluster_hardware",
        "description": "Analyze cluster hardware topology from must-gather data.",
        ...
      },
      ...
    ]
  }
}
```

---

## Method 4: End-to-End Workflow Testing

**Complete realistic scenario** from requirement to PPC command.

### Run Workflow Test
```bash
# Without must-gather (limited functionality)
./test_workflow.sh

# With must-gather (full functionality)
./test_workflow.sh performance-profile-creator-mcp:latest /path/to/must-gather
```

### Workflow Steps
1. 📋 List workload templates
2. 🧠 Get recommendations from natural language
3. 🔍 Analyze cluster hardware
4. ✅ Validate requirements
5. 🛠️ Generate PPC command

### Sample Output
```
╔════════════════════════════════════════════════════════════╗
║  Performance Profile Creator - End-to-End Workflow Test   ║
╚════════════════════════════════════════════════════════════╝

📋 Configuration:
  Image: performance-profile-creator-mcp:latest
  Must-gather: /path/to/must-gather
  Output dir: /tmp/ppc-mcp-test-1234567890

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Step 1: Discover Available Workload Templates
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

Available templates:
  • 5g-ran: 5G RAN (Radio Access Network)
  • telco-vnf: Telco VNF (Virtual Network Function)
  • database: Database Server
  ...

━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━
Step 2: Get Recommendations from Natural Language
━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━

User requirement: "I need to run 5G RAN workloads with ultra-low latency"

Recommendations:
  Workload Type: 5g-ran
  Reasoning: Detected 5G/RAN workload - requires ultra-low latency
  ...

✅ Workflow Test Completed
```

---

## Method 5: Cursor Integration Testing

**Test in real MCP client** (Cursor, Claude Desktop, etc.)

### Step 1: Configure Cursor

1. Open Cursor Settings
2. Navigate to: **Features > MCP Servers**
3. Add this configuration:

```json
{
  "mcpServers": {
    "performance-profile-creator": {
      "command": "podman",
      "args": [
        "run",
        "-i",
        "--rm",
        "-v", "/absolute/path/to/must-gather:/must-gather:z",
        "-e", "MCP_TRANSPORT",
        "localhost/performance-profile-creator-mcp:latest"
      ],
      "env": {
        "MCP_TRANSPORT": "stdio"
      }
    }
  }
}
```

**Important:** Replace `/absolute/path/to/must-gather` with your actual path.

### Step 2: Restart Cursor

### Step 3: Test with Natural Language

In a Cursor chat, try:

```
"List available workload templates for performance profiles"
```

Expected: MCP server responds with list of 7 templates

```
"I need to create a performance profile for 5G RAN workloads"
```

Expected: MCP server provides recommendations and guidance

```
"Analyze the hardware from my must-gather data"
```

Expected: MCP server parses and summarizes cluster hardware

### Step 4: Interactive Workflow

```
User: "I need a performance profile for 5G RAN"
↓
AI (via MCP): Lists templates, recommends 5g-ran
↓
User: "Generate the configuration"
↓
AI (via MCP): Analyzes hardware, validates, generates PPC command
↓
User: "Explain the parameters"
↓
AI (via MCP): Provides detailed explanation of each parameter
```

---

## Common Issues & Solutions

### Issue 1: Container Build Fails

**Problem:** UBI9 base image pull fails

**Solution:**
```bash
# Login to Red Hat registry
podman login registry.redhat.io

# Or use alternative base image
# Edit Containerfile: FROM python:3.11-slim
```

### Issue 2: Must-Gather Not Found

**Problem:** `Error: No hardware data available`

**Solution:**
```bash
# Check path is absolute
pwd
ls -la /absolute/path/to/must-gather

# Check volume mount syntax
-v /absolute/path:/must-gather:z
   ^^^^^^^^^^^^^^^^  ^^^^^^^^^^^^
   host path         container path
```

### Issue 3: Permission Denied

**Problem:** `Permission denied` when reading must-gather

**Solution:**
```bash
# Add :z to volume mount for SELinux
-v /path/to/must-gather:/must-gather:z
                                    ^^

# Or temporarily disable SELinux (not recommended)
sudo setenforce 0
```

### Issue 4: MCP Client Can't Connect

**Problem:** Cursor shows "MCP server failed to start"

**Solution:**
```bash
# Test container manually first
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  podman run -i --rm \
    -v /path/to/must-gather:/must-gather:z \
    -e MCP_TRANSPORT=stdio \
    performance-profile-creator-mcp:latest

# Check Cursor logs
# Cursor > Help > Show Logs > MCP
```

### Issue 5: Python Import Errors (Local Testing)

**Problem:** `ModuleNotFoundError: No module named 'fastmcp'`

**Solution:**
```bash
# Install dependencies
pip install -r requirements.txt

# Or use virtual environment
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
```

---

## Collecting a Must-Gather

If you don't have must-gather data yet:

```bash
# Collect must-gather from your OpenShift cluster
oc adm must-gather \
  --image=quay.io/openshift-kni/performance-addon-operator-must-gather:4.11 \
  --dest-dir=./must-gather-output

# Extract if compressed
tar -xzf must-gather.tar.gz
```

---

## Test Checklist

Before considering testing complete:

- [ ] Local Python tests pass with mock data
- [ ] Local Python tests pass with real must-gather
- [ ] Container builds successfully
- [ ] MCP protocol tests all return valid JSON
- [ ] End-to-end workflow completes without errors
- [ ] Cursor (or MCP client) can connect to server
- [ ] Can list workload templates via client
- [ ] Can get recommendations from natural language
- [ ] Can analyze real must-gather data
- [ ] Can validate requirements against hardware
- [ ] Can generate PPC commands
- [ ] Generated commands are syntactically correct

---

## Performance Benchmarks

Approximate execution times:

| Test | Time | Notes |
|------|------|-------|
| Local Python | ~2 sec | Fastest, uses mock data |
| Container build | ~2 min | One-time setup |
| MCP protocol | ~5 sec | Per test request |
| End-to-end workflow | ~10 sec | Full workflow |
| Cursor integration | ~3 sec | Per interaction |

---

## Next Steps

After successful testing:

1. **Production Deployment**
   - Push image to registry: `podman push <image> <registry>`
   - Update MCP config with registry URL
   
2. **Team Rollout**
   - Share MCP configuration with team
   - Provide training on workload templates
   - Document organization-specific best practices

3. **Integration**
   - Integrate with CI/CD pipelines
   - Add to GitOps workflows
   - Create organization-specific templates

4. **Monitoring**
   - Track which templates are used
   - Collect feedback on recommendations
   - Iterate on validation rules

---

## Support

- **Documentation**: See README.md for usage, ARCHITECTURE.md for internals
- **Issues**: Report in cluster-node-tuning-operator repo
- **Questions**: Check PROJECT_SUMMARY.md for overview




