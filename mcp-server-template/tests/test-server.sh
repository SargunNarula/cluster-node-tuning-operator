#!/bin/bash
# Quick test script for the MCP server

set -e

echo "🚀 Performance Profile Creator MCP Server Test"
echo "=============================================="
echo

# Check if must-gather path is provided
if [ -z "$1" ]; then
    echo "Usage: $0 <path-to-must-gather>"
    echo
    echo "Example:"
    echo "  $0 /path/to/must-gather"
    exit 1
fi

MUST_GATHER_PATH="$1"

if [ ! -d "$MUST_GATHER_PATH" ]; then
    echo "❌ Error: Must-gather directory not found: $MUST_GATHER_PATH"
    exit 1
fi

echo "📁 Using must-gather: $MUST_GATHER_PATH"
echo

# Build the container if it doesn't exist
if ! podman images | grep -q "performance-profile-creator-mcp"; then
    echo "🔨 Building container image..."
    podman build -t performance-profile-creator-mcp:latest .
    echo "✅ Container built successfully"
    echo
fi

# Run a simple test
echo "🧪 Testing MCP server..."
echo

# Create a test input for analyzing hardware
TEST_INPUT=$(cat <<EOF
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "analyze_cluster_hardware",
    "arguments": {
      "must_gather_path": "/must-gather"
    }
  }
}
EOF
)

echo "📤 Sending test request: analyze_cluster_hardware"
echo "$TEST_INPUT" | podman run -i --rm \
  -v "$MUST_GATHER_PATH:/must-gather:z" \
  -e MCP_TRANSPORT=stdio \
  performance-profile-creator-mcp:latest

echo
echo "✅ Test completed!"
echo
echo "To use this MCP server with Cursor or other MCP clients:"
echo "1. Copy example-config.json to your MCP client configuration"
echo "2. Update the must-gather path in the configuration"
echo "3. Start using natural language to create performance profiles"




