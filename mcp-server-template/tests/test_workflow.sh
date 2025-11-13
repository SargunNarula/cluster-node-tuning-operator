#!/bin/bash
# End-to-end workflow test with realistic scenario

set -e

# Color codes
GREEN='\033[0;32m'
YELLOW='\033[1;33m'
BLUE='\033[0;34m'
RED='\033[0;31m'
NC='\033[0m' # No Color

echo -e "${BLUE}╔════════════════════════════════════════════════════════════╗${NC}"
echo -e "${BLUE}║  Performance Profile Creator - End-to-End Workflow Test   ║${NC}"
echo -e "${BLUE}╚════════════════════════════════════════════════════════════╝${NC}"
echo

# Configuration
IMAGE_NAME="${1:-performance-profile-creator-mcp:latest}"
MUST_GATHER_PATH="${2}"
OUTPUT_DIR="/tmp/ppc-mcp-test-$(date +%s)"

echo -e "${YELLOW}📋 Configuration:${NC}"
echo "  Image: $IMAGE_NAME"
echo "  Must-gather: ${MUST_GATHER_PATH:-<using mock data>}"
echo "  Output dir: $OUTPUT_DIR"
echo

# Check prerequisites
echo -e "${YELLOW}🔍 Checking prerequisites...${NC}"

if ! command -v podman &> /dev/null; then
    echo -e "${RED}❌ podman not found${NC}"
    exit 1
fi
echo -e "${GREEN}✓ podman found${NC}"

if ! command -v jq &> /dev/null; then
    echo -e "${YELLOW}⚠ jq not found (optional, for pretty JSON)${NC}"
fi

if ! podman images | grep -q "$(echo $IMAGE_NAME | cut -d: -f1)"; then
    echo -e "${RED}❌ Image $IMAGE_NAME not found${NC}"
    echo "   Build it first: podman build -t $IMAGE_NAME ."
    exit 1
fi
echo -e "${GREEN}✓ Image found${NC}"

if [ -n "$MUST_GATHER_PATH" ] && [ ! -d "$MUST_GATHER_PATH" ]; then
    echo -e "${RED}❌ Must-gather path does not exist: $MUST_GATHER_PATH${NC}"
    exit 1
fi

mkdir -p "$OUTPUT_DIR"
echo -e "${GREEN}✓ Output directory created${NC}"
echo

# Function to call MCP tool
call_mcp_tool() {
    local tool_name="$1"
    local arguments="$2"
    local output_file="$3"
    
    local request=$(cat <<EOF
{
  "jsonrpc": "2.0",
  "id": 1,
  "method": "tools/call",
  "params": {
    "name": "$tool_name",
    "arguments": $arguments
  }
}
EOF
)
    
    # Only mount volume if MUST_GATHER_PATH is provided and exists
    if [ -n "$MUST_GATHER_PATH" ] && [ -d "$MUST_GATHER_PATH" ]; then
        echo "$request" | podman run -i --rm \
            -v "$MUST_GATHER_PATH:/must-gather:z" \
            -e MCP_TRANSPORT=stdio \
            "$IMAGE_NAME" > "$output_file" 2>&1
    else
        echo "$request" | podman run -i --rm \
            -e MCP_TRANSPORT=stdio \
            "$IMAGE_NAME" > "$output_file" 2>&1
    fi
}

# Workflow Step 1: List available workload templates
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Step 1: Discover Available Workload Templates${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo

call_mcp_tool "list_workload_templates" "{}" "$OUTPUT_DIR/step1_templates.json"

echo "Available templates:"
if command -v jq &> /dev/null; then
    cat "$OUTPUT_DIR/step1_templates.json" | jq -r '.result.content[0].text | fromjson | .templates[] | "  • \(.type): \(.name)"' 2>/dev/null || cat "$OUTPUT_DIR/step1_templates.json"
else
    grep -o '"type":"[^"]*"' "$OUTPUT_DIR/step1_templates.json" | cut -d'"' -f4 | sed 's/^/  • /' || echo "  (see $OUTPUT_DIR/step1_templates.json)"
fi
echo

# Workflow Step 2: Get recommendations from natural language
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Step 2: Get Recommendations from Natural Language${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo

USER_REQUIREMENT="I need to run 5G RAN workloads with ultra-low latency and DPDK packet processing"
echo "User requirement: \"$USER_REQUIREMENT\""
echo

call_mcp_tool "get_workload_recommendations" "{\"workload_description\": \"$USER_REQUIREMENT\"}" "$OUTPUT_DIR/step2_recommendations.json"

echo "Recommendations:"
if command -v jq &> /dev/null; then
    cat "$OUTPUT_DIR/step2_recommendations.json" | jq -r '.result.content[0].text | fromjson | .recommendations | "  Workload Type: \(.workload_type)\n  Reasoning: \(.reasoning[0] // "N/A")"' 2>/dev/null || cat "$OUTPUT_DIR/step2_recommendations.json"
else
    echo "  (see $OUTPUT_DIR/step2_recommendations.json)"
fi
echo

# Workflow Step 3: Analyze cluster hardware (if must-gather provided)
if [ -n "$MUST_GATHER_PATH" ]; then
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo -e "${YELLOW}Step 3: Analyze Cluster Hardware${NC}"
    echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
    echo

    call_mcp_tool "analyze_cluster_hardware" "{\"must_gather_path\": \"/must-gather\"}" "$OUTPUT_DIR/step3_hardware.json"

    echo "Hardware summary:"
    if command -v jq &> /dev/null; then
        cat "$OUTPUT_DIR/step3_hardware.json" | jq -r '.result.content[0].text | fromjson | .summary' 2>/dev/null || echo "  (see $OUTPUT_DIR/step3_hardware.json)"
    else
        echo "  (see $OUTPUT_DIR/step3_hardware.json)"
    fi
    echo
else
    echo -e "${YELLOW}⚠ Skipping hardware analysis (no must-gather provided)${NC}"
    echo
fi

# Workflow Step 4: Validate requirements
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Step 4: Validate Performance Requirements${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo

VALIDATION_ARGS=$(cat <<EOF
{
  "workload_type": "5g-ran",
  "reserved_cpu_count": 8,
  "enable_rt_kernel": true,
  "enable_dpdk": true,
  "power_mode": "ultra-low-latency"
}
EOF
)

if [ -n "$MUST_GATHER_PATH" ]; then
    call_mcp_tool "validate_performance_requirements" "$VALIDATION_ARGS" "$OUTPUT_DIR/step4_validation.json"
    
    echo "Validation result:"
    if command -v jq &> /dev/null; then
        cat "$OUTPUT_DIR/step4_validation.json" | jq -r '.result.content[0].text | fromjson | .validation | "  Status: \(.overall_status)\n  Errors: \(.errors | length)\n  Warnings: \(.warnings | length)\n  Recommendations: \(.recommendations | length)"' 2>/dev/null || echo "  (see $OUTPUT_DIR/step4_validation.json)"
    else
        echo "  (see $OUTPUT_DIR/step4_validation.json)"
    fi
    echo
else
    echo -e "${YELLOW}⚠ Skipping validation (no must-gather provided)${NC}"
    echo
fi

# Workflow Step 5: Generate PPC command
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${YELLOW}Step 5: Generate PPC Command${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo

COMMAND_ARGS=$(cat <<EOF
{
  "workload_type": "5g-ran",
  "mcp_name": "worker-cnf",
  "profile_name": "5g-performance",
  "enable_rt_kernel": true,
  "enable_dpdk": true,
  "power_mode": "ultra-low-latency"
}
EOF
)

if [ -n "$MUST_GATHER_PATH" ]; then
    call_mcp_tool "generate_ppc_command" "$COMMAND_ARGS" "$OUTPUT_DIR/step5_command.json"
    
    echo "Generated PPC command:"
    if command -v jq &> /dev/null; then
        cat "$OUTPUT_DIR/step5_command.json" | jq -r '.result.content[0].text | fromjson | .command' 2>/dev/null || echo "  (see $OUTPUT_DIR/step5_command.json)"
    else
        echo "  (see $OUTPUT_DIR/step5_command.json)"
    fi
    echo
    
    echo "Explanation:"
    if command -v jq &> /dev/null; then
        cat "$OUTPUT_DIR/step5_command.json" | jq -r '.result.content[0].text | fromjson | .explanation' 2>/dev/null | head -20
    else
        echo "  (see $OUTPUT_DIR/step5_command.json)"
    fi
    echo
else
    echo -e "${YELLOW}⚠ Skipping command generation (no must-gather provided)${NC}"
    echo
fi

# Summary
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo -e "${GREEN}✅ Workflow Test Completed${NC}"
echo -e "${BLUE}━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━━${NC}"
echo
echo "Test results saved to: $OUTPUT_DIR"
echo
echo "Files created:"
ls -lh "$OUTPUT_DIR"
echo
echo -e "${YELLOW}💡 Next steps:${NC}"
echo "  1. Review the output files in $OUTPUT_DIR"
echo "  2. Configure the MCP server in your MCP client (e.g., Cursor)"
echo "  3. Test with real must-gather data for production use"
echo


