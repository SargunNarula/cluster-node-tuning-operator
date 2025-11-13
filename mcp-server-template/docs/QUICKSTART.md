# Quick Start Guide - 5 Minutes to Running MCP Server

## 🎯 Goal
Get the Performance Profile Creator MCP Server running in 5 minutes.

## ✅ Prerequisites Check
```bash
which podman python3 jq
# All should return paths
```

## 🚀 3-Step Quick Start

### Step 1: Test Locally (30 seconds)
```bash
cd mcp-server-template
python3 test_local.py
```
  
**Expected:** All tests pass with green checkmarks ✅

### Step 2: Build Container (2 minutes)
```bash
podman build -t performance-profile-creator-mcp:latest .
```

**Expected:** `Successfully tagged localhost/performance-profile-creator-mcp:latest`

**Note:** Uses public `python:3.11-slim` image (no authentication required).
See `BUILD_NOTES.md` if you need the Red Hat UBI9 version.

### Step 3: Test Container (30 seconds)
```bash
# Quick test - no SELinux issues, uses mock data
./test_simple.sh performance-profile-creator-mcp:latest
```

**Expected:** All 3 tests pass, showing 6 available tools

**Alternative:** Manual protocol test:
```bash
echo '{"jsonrpc":"2.0","id":1,"method":"tools/list"}' | \
  podman run -i --rm \
    -e MCP_TRANSPORT=stdio \
    performance-profile-creator-mcp:latest | jq .
```

## 🎓 What You Can Do Now

### Option A: Use with Cursor

1. **Configure Cursor** (`Cursor Settings` > `Features` > `MCP Servers`):
```json
{
  "mcpServers": {
    "performance-profile-creator": {
      "command": "podman",
      "args": [
        "run", "-i", "--rm",
        "-v", "/path/to/must-gather:/must-gather:z",
        "-e", "MCP_TRANSPORT",
        "localhost/performance-profile-creator-mcp:latest"
      ],
      "env": {"MCP_TRANSPORT": "stdio"}
    }
  }
}
```

2. **Replace** `/path/to/must-gather` with your actual path

3. **Restart Cursor**

4. **Try in chat:**
   ```
   "List available workload templates"
   "I need a performance profile for 5G RAN"
   "Analyze my cluster hardware"
   ```

### Option B: Command Line Testing

```bash
# Test complete workflow
./test_workflow.sh performance-profile-creator-mcp:latest /path/to/must-gather

# Or test MCP protocol
./test_mcp_protocol.sh performance-profile-creator-mcp:latest /path/to/must-gather
```

## 📝 Example Interactions

### Get Recommendations
```bash
echo '{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "get_workload_recommendations",
    "arguments": {
      "workload_description": "5G RAN with ultra-low latency"
    }
  }
}' | podman run -i --rm \
  -e MCP_TRANSPORT=stdio \
  performance-profile-creator-mcp:latest | jq .
```

### List Templates
```bash
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

## 🆘 Troubleshooting

### Container build fails
```bash
# Login to Red Hat registry
podman login registry.redhat.io
```

### Permission denied
```bash
# Add :z to volume mount
-v /path:/must-gather:z
                    ^^
```

### Can't find must-gather
```bash
# Collect from cluster
oc adm must-gather \
  --image=quay.io/openshift-kni/performance-addon-operator-must-gather:4.11 \
  --dest-dir=./must-gather
```

### Python module errors
```bash
pip install -r requirements.txt
```

## 📚 Full Documentation

- **Testing Guide**: See `TESTING.md` for comprehensive testing
- **Usage**: See `README.md` for detailed usage
- **Architecture**: See `ARCHITECTURE.md` for internals
- **Summary**: See `PROJECT_SUMMARY.md` for overview

## 💡 Key Commands Reference

| Task | Command |
|------|---------|
| Test locally | `./test_local.py` |
| Build container | `podman build -t ppc-mcp:latest .` |
| Test protocol | `./test_mcp_protocol.sh` |
| Test workflow | `./test_workflow.sh ppc-mcp:latest /must-gather` |
| Run container | `podman run -i --rm -e MCP_TRANSPORT=stdio ppc-mcp:latest` |

## 🎉 Next Steps

1. ✅ **Verified**: Local tests pass
2. ✅ **Built**: Container image created
3. ✅ **Tested**: MCP protocol works
4. 📝 **Configure**: Add to Cursor/MCP client
5. 🚀 **Use**: Create performance profiles!

## 🔗 Support

- **Questions**: Check `TESTING.md` and `README.md`
- **Issues**: Report in cluster-node-tuning-operator repo
- **Examples**: See test scripts for usage patterns


