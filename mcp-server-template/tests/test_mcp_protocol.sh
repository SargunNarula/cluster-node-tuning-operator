#!/bin/bash
# Test MCP server protocol directly via stdio

set -e

echo "🧪 MCP Protocol Testing"
echo "======================="
echo

# Color codes
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
RED='\033[0;31m'
NC='\033[0m' # No Color

# Function to send MCP request and capture response
send_mcp_request() {
    local request="$1"
    local description="$2"
    
    echo -e "${YELLOW}📤 Test: $description${NC}"
    echo "Request:"
    echo "$request" | jq '.' 2>/dev/null || echo "$request"
    echo
    
    echo "Response:"
    # Compact JSON to single line (MCP protocol requires newline-delimited JSON)
    local compact_request=$(echo "$request" | jq -c '.' 2>/dev/null || echo "$request")
    
    # Only mount volume if MUST_GATHER_PATH is provided and exists
    if [ -n "$MUST_GATHER_PATH" ] && [ -d "$MUST_GATHER_PATH" ]; then
        echo "$compact_request" | podman run -i --rm \
            -v "$MUST_GATHER_PATH:/must-gather:z" \
            -e MCP_TRANSPORT=stdio \
            "$IMAGE_NAME" 2>&1 | head -50
    else
        echo "$compact_request" | podman run -i --rm \
            -e MCP_TRANSPORT=stdio \
            "$IMAGE_NAME" 2>&1 | head -50
    fi
    echo
    echo "---"
    echo
}

# Check if image name is provided
IMAGE_NAME="${1:-performance-profile-creator-mcp:latest}"
MUST_GATHER_PATH="${2}"  # Don't default to /tmp

echo "Using image: $IMAGE_NAME"
if [ -n "$MUST_GATHER_PATH" ]; then
    echo "Must-gather path: $MUST_GATHER_PATH"
else
    echo "Must-gather path: <none, using mock data>"
fi
echo

# Check if image exists
if ! podman images | grep -q "$(echo $IMAGE_NAME | cut -d: -f1)"; then
    echo -e "${RED}❌ Error: Image $IMAGE_NAME not found${NC}"
    echo "   Build it first: podman build -t $IMAGE_NAME ."
    exit 1
fi

echo -e "${GREEN}✓ Image found${NC}"
echo

# Test 1: Initialize and list tools
echo -e "${YELLOW}📤 Test: Initialize MCP Session and List Tools${NC}"
echo

if [ -n "$MUST_GATHER_PATH" ] && [ -d "$MUST_GATHER_PATH" ]; then
    cat << 'EOF' | podman run -i --rm \
        -v "$MUST_GATHER_PATH:/must-gather:z" \
        -e MCP_TRANSPORT=stdio \
        "$IMAGE_NAME" 2>&1 | grep -A 30 "tools"
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
EOF
else
    cat << 'EOF' | podman run -i --rm \
        -e MCP_TRANSPORT=stdio \
        "$IMAGE_NAME" 2>&1 | grep -A 30 "tools"
{"jsonrpc":"2.0","id":1,"method":"initialize","params":{"protocolVersion":"2024-11-05","capabilities":{},"clientInfo":{"name":"test","version":"1.0"}}}
{"jsonrpc":"2.0","id":2,"method":"tools/list"}
EOF
fi

echo
echo "---"
echo

# Test 2: List workload templates
send_mcp_request '{
  "jsonrpc": "2.0",
  "id": 2,
  "method": "tools/call",
  "params": {
    "name": "list_workload_templates",
    "arguments": {}
  }
}' "List Workload Templates"

# Test 3: Get workload recommendations
send_mcp_request '{
  "jsonrpc": "2.0",
  "id": 3,
  "method": "tools/call",
  "params": {
    "name": "get_workload_recommendations",
    "arguments": {
      "workload_description": "I need to run 5G RAN workloads with ultra-low latency"
    }
  }
}' "Get Workload Recommendations (Natural Language)"

# Test 4: List resources
send_mcp_request '{
  "jsonrpc": "2.0",
  "id": 4,
  "method": "resources/list"
}' "List Available Resources"

# Test 5: List prompts
send_mcp_request '{
  "jsonrpc": "2.0",
  "id": 5,
  "method": "prompts/list"
}' "List Available Prompts"

echo -e "${GREEN}✅ MCP Protocol tests completed${NC}"
echo
if [ -z "$MUST_GATHER_PATH" ]; then
    echo "💡 Note: Tests ran without must-gather data (using mock data)"
    echo
    echo "To test with real must-gather:"
    echo "  ./test_mcp_protocol.sh $IMAGE_NAME /path/to/must-gather"
fi
echo
echo "To test full workflow:"
echo "  ./test_workflow.sh $IMAGE_NAME /path/to/must-gather"


