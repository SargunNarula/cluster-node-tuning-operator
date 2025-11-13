# How to Run and Test the MCP Server - Complete Guide

## 📁 What You Have

Your `mcp-server-template/` directory now contains:

### Core Implementation (5 files)
- `mcp_server.py` - Main MCP server with 6 tools
- `must_gather_parser.py` - Parses cluster hardware from must-gather
- `hardware_validator.py` - Validates requirements against hardware
- `ppc_generator.py` - Generates PPC commands
- `workload_templates.py` - 7 pre-configured workload templates

### Configuration Files (3 files)
- `requirements.txt` - Python dependencies
- `Containerfile` - Container image definition
- `example-config.json` - MCP client configuration template

### Test Scripts (4 files)
- `test_local.py` - Fast local testing with mock data
- `test_mcp_protocol.sh` - MCP protocol compliance testing
- `test_workflow.sh` - End-to-end workflow testing
- `test-server.sh` - Simple server validation

### Documentation (5 files)
- `QUICKSTART.md` - 5-minute quick start
- `TESTING.md` - Comprehensive testing guide
- `README.md` - User documentation
- `ARCHITECTURE.md` - Technical architecture
- `PROJECT_SUMMARY.md` - Project overview

---

## 🎯 Choose Your Path

### Path 1: Just Want to Test Quickly? → Use This

```bash
cd mcp-server-template

# Install Python dependencies (one time)
pip3 install -r requirements.txt

# Run local tests (uses mock data)
python3 test_local.py

# ✅ If tests pass, you're done!
```

**What it tests:** All components work correctly with mock hardware data.

**Time:** 5 seconds

---

### Path 2: Want to Test with Real Cluster Data? → Use This

**Prerequisites:** You need must-gather data from your OpenShift cluster.

#### Collect Must-Gather (if you don't have it)
```bash
# On a machine with oc CLI and cluster access
oc adm must-gather \
  --image=quay.io/openshift-kni/performance-addon-operator-must-gather:4.11 \
  --dest-dir=./must-gather-output

# Copy to your development machine
# Note the absolute path
pwd  # e.g., /home/snarula/must-gather-output
```

#### Test with Real Data
```bash
cd mcp-server-template

# Test with your must-gather
python3 test_local.py /absolute/path/to/must-gather-output

# ✅ Should parse real hardware topology
```

**What it tests:** Parser works with real cluster data, validation uses actual hardware.

**Time:** 10 seconds

---

### Path 3: Want to Deploy the Container? → Use This

```bash
cd mcp-server-template

# Step 1: Build container image
podman build -t performance-profile-creator-mcp:latest .
# ⏱ Takes ~2 minutes

# Step 2: Verify build
podman images | grep performance-profile-creator-mcp

# Step 3: Quick smoke test
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  podman run -i --rm \
    -e MCP_TRANSPORT=stdio \
    performance-profile-creator-mcp:latest
# ✅ Should return JSON with 6 tools

# Step 4: Test with real must-gather
./test_workflow.sh performance-profile-creator-mcp:latest /path/to/must-gather
# ⏱ Takes ~10 seconds
```

**What it tests:** Full container deployment, all tools via MCP protocol.

**Time:** 3 minutes

---

### Path 4: Want to Use in Cursor? → Use This

**Prerequisites:** Container built (see Path 3)

#### Step 1: Configure Cursor

1. Open **Cursor Settings**
2. Go to **Features** → **MCP Servers**
3. Click **Edit Config** (opens JSON file)
4. Add this configuration:

```json
{
  "mcpServers": {
    "performance-profile-creator": {
      "command": "podman",
      "args": [
        "run",
        "-i",
        "--rm",
        "-v", "/ABSOLUTE/PATH/TO/must-gather:/must-gather:z",
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

5. **IMPORTANT:** Replace `/ABSOLUTE/PATH/TO/must-gather` with your actual path
   ```bash
   # Get absolute path
   cd /path/to/must-gather
   pwd
   # Use the output in your config
   ```

6. Save and **Restart Cursor**

#### Step 2: Test in Cursor

Open a new chat and try:

```
"List available workload templates"
```

Expected response: List of 7 templates (5g-ran, telco-vnf, database, etc.)

```
"I need to create a performance profile for 5G RAN workloads"
```

Expected response: Recommendations with RT kernel, DPDK, ultra-low-latency settings

```
"Analyze my cluster hardware"
```

Expected response: Hardware summary from your must-gather

#### Step 3: Full Workflow

```
You: "I need a performance profile for 5G RAN with ultra-low latency"

AI (via MCP):
  ✓ Analyzed your cluster: 48 CPUs, 2 NUMA nodes
  ✓ Identified workload: 5g-ran
  ✓ Recommends: RT kernel, DPDK, disable HT
  ✓ Validation: Configuration is feasible
  ✓ Generated PPC command with explanations

You: "Explain why disable hyperthreading?"

AI (via MCP):
  Disabling HT reduces available CPUs by 50% but:
  • Improves deterministic latency
  • Eliminates sibling thread interference
  • Critical for 5G timing requirements
  ...

You: "Generate the profile"

AI (via MCP):
  [Creates performance-profile.yaml]
  To apply: oc apply -f performance-profile.yaml
```

---

## 🧪 Test Scripts Explained

### `test_local.py` - Component Testing
**What:** Tests individual modules without container  
**When:** Development, quick iteration  
**How:** `python3 test_local.py [must-gather-path]`  
**Output:** Pass/fail for each component

### `test_mcp_protocol.sh` - Protocol Testing
**What:** Tests MCP protocol compliance  
**When:** After container build  
**How:** `./test_mcp_protocol.sh [image-name] [must-gather-path]`  
**Output:** MCP JSON-RPC requests/responses

### `test_workflow.sh` - End-to-End Testing
**What:** Complete workflow from requirements to PPC command  
**When:** Before deployment  
**How:** `./test_workflow.sh [image-name] [must-gather-path]`  
**Output:** Step-by-step workflow results

### `test-server.sh` - Simple Validation
**What:** Basic server functionality check  
**When:** Quick sanity check  
**How:** `./test-server.sh /must-gather-path`  
**Output:** Server responds correctly

---

## 📊 Testing Matrix

| What to Test | Script | Must-Gather Needed? | Time |
|--------------|--------|---------------------|------|
| Components work | `test_local.py` | No (uses mock) | 5s |
| Real data parsing | `test_local.py /path` | Yes | 10s |
| Container builds | `podman build` | No | 2m |
| MCP protocol | `test_mcp_protocol.sh` | Optional | 5s |
| Full workflow | `test_workflow.sh` | Yes (recommended) | 10s |
| Cursor integration | Manual in Cursor | Yes | 1m |

---

## 🔍 Debugging Tips

### Check Container Logs
```bash
# Run with output visible
podman run -i --rm \
  -v /must-gather:/must-gather:z \
  -e MCP_TRANSPORT=stdio \
  performance-profile-creator-mcp:latest < test_request.json

# Check container filesystem
podman run -it --rm --entrypoint /bin/bash \
  performance-profile-creator-mcp:latest
```

### Test Individual Tools
```bash
# Test list_workload_templates
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "list_workload_templates",
    "arguments": {}
  }
}' | podman run -i --rm \
  -e MCP_TRANSPORT=stdio \
  performance-profile-creator-mcp:latest | jq .
```

### Check Python Imports
```bash
# In container
podman run -it --rm --entrypoint python3 \
  performance-profile-creator-mcp:latest \
  -c "import mcp_server; print('OK')"
```

### Verify Must-Gather Mount
```bash
# Check if must-gather is accessible in container
podman run -it --rm \
  -v /path/to/must-gather:/must-gather:z \
  performance-profile-creator-mcp:latest \
  ls -la /must-gather
```

---

## 🚨 Common Issues & Fixes

### Issue: `ModuleNotFoundError`
**Cause:** Dependencies not installed  
**Fix:**
```bash
pip3 install -r requirements.txt
# Or create virtual environment
python3 -m venv venv
source venv/bin/activate
pip install -r requirements.txt
```

### Issue: `Permission denied` reading must-gather
**Cause:** SELinux blocking volume mount  
**Fix:**
```bash
# Add :z to volume mount
-v /path:/must-gather:z
                    ^^
# Or temporarily disable SELinux
sudo setenforce 0
```

### Issue: Container build fails downloading UBI9
**Cause:** Not logged into Red Hat registry  
**Fix:**
```bash
podman login registry.redhat.io
# Enter Red Hat credentials
```

### Issue: Cursor can't connect to MCP server
**Cause:** Path or config error  
**Fix:**
```bash
# 1. Test manually first
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  podman run -i --rm \
    -v /path/to/must-gather:/must-gather:z \
    -e MCP_TRANSPORT=stdio \
    performance-profile-creator-mcp:latest

# 2. Check Cursor logs
# Cursor > Help > Show Logs > MCP

# 3. Verify absolute paths in config
# Must be absolute, not relative!
```

### Issue: `No hardware data available`
**Cause:** Must-gather path incorrect or not mounted  
**Fix:**
```bash
# Verify path exists
ls -la /absolute/path/to/must-gather

# Check it has expected structure
ls /absolute/path/to/must-gather/*/cluster-scoped-resources/core/nodes/

# Use absolute path in config, not relative
```

---

## ✅ Success Checklist

Before considering setup complete:

- [ ] `test_local.py` passes with mock data
- [ ] `test_local.py /must-gather` passes with real data
- [ ] Container builds successfully
- [ ] `test_mcp_protocol.sh` all tests pass
- [ ] `test_workflow.sh` completes end-to-end
- [ ] Cursor/MCP client connects successfully
- [ ] Can list templates in Cursor
- [ ] Can get recommendations from natural language
- [ ] Can analyze real must-gather
- [ ] Can validate requirements
- [ ] Can generate PPC commands
- [ ] Generated commands look correct

---

## 📈 Next Steps After Testing

### For Development
1. Modify templates in `workload_templates.py`
2. Add validation rules in `hardware_validator.py`
3. Enhance parsing in `must_gather_parser.py`
4. Test changes with `python3 test_local.py`

### For Deployment
1. Push to registry: `podman push <image> <registry>`
2. Update MCP configs with registry URL
3. Share with team
4. Document org-specific usage patterns

### For Integration
1. Add to CI/CD pipelines
2. Create GitOps workflows
3. Build organization-specific templates
4. Set up monitoring/feedback

---

## 📚 Further Reading

- **Quick Start:** `QUICKSTART.md` - 5-minute setup
- **Testing:** `TESTING.md` - Comprehensive testing guide
- **Usage:** `README.md` - User documentation
- **Architecture:** `ARCHITECTURE.md` - How it works
- **Summary:** `PROJECT_SUMMARY.md` - Project overview

---

## 🎉 You're Ready!

You now have:
- ✅ Fully functional MCP server
- ✅ Comprehensive test suite
- ✅ Multiple testing methods
- ✅ Clear documentation
- ✅ Debugging tools

**Start with:** `python3 test_local.py` and work your way up!




