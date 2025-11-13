# Performance Profile Creator MCP Server

An intelligent MCP (Model Context Protocol) server that wraps the OpenShift Performance Profile Creator (PPC) tool. This server understands workload requirements in natural language, validates them against cluster hardware capabilities, and generates optimized Performance Profile configurations.

## Features

- 🔍 **Hardware Analysis**: Parse must-gather data to understand cluster topology
- ✅ **Validation**: Check if performance requirements are feasible for your hardware
- 🎯 **Workload Templates**: Pre-configured templates for common use cases (5G RAN, Telco VNF, Database, AI/ML, etc.)
- 🤖 **Natural Language**: Describe your workload and get intelligent recommendations
- 🛠️ **Command Generation**: Automatically generate optimized PPC commands
- 📝 **Explanation**: Understand what each parameter does and why it's recommended

## Architecture

```
┌─────────────────────┐
│  User/AI Assistant  │
└──────────┬──────────┘
           │ MCP Protocol
           ▼
┌─────────────────────┐
│   MCP Server        │
│  (mcp_server.py)    │
├─────────────────────┤
│ • analyze_cluster   │
│ • validate_reqs     │
│ • generate_command  │
│ • create_profile    │
└──────────┬──────────┘
           │
    ┌──────┴──────┬──────────┬──────────────┐
    ▼             ▼          ▼              ▼
┌─────────┐ ┌──────────┐ ┌──────┐ ┌────────────┐
│Must-    │ │Hardware  │ │PPC   │ │Workload    │
│Gather   │ │Validator │ │Gen   │ │Templates   │
│Parser   │ │          │ │      │ │            │
└─────────┘ └──────────┘ └──────┘ └────────────┘
```

## Quick Start

### 1. Building locally

Build the container image using Podman:

```bash
cd mcp-server-template
podman build -t performance-profile-creator-mcp:latest .

# Test it (avoids SELinux issues)
./test_simple.sh performance-profile-creator-mcp:latest
```

### 2. Running the MCP Server

#### Option A: Run with Podman/Docker

```bash
podman run -i --rm \
  -v /path/to/must-gather:/must-gather:z \
  -e MCP_TRANSPORT=stdio \
  performance-profile-creator-mcp:latest
```

#### Option B: Configure in MCP Client (e.g., Cursor, Claude Desktop)

Add to your MCP configuration file:

```json
{
  "mcpServers": {
    "performance-profile-creator": {
      "command": "podman",
      "args": [
        "run",
        "-i",
        "--rm",
        "-v", "/path/to/must-gather:/must-gather:z",
        "-e", "MCP_TRANSPORT",
        "performance-profile-creator-mcp:latest"
      ],
      "env": {
        "MCP_TRANSPORT": "stdio"
      }
    }
  }
}
```

## Available Tools

### 1. `analyze_cluster_hardware`

Analyze cluster hardware topology from must-gather data.

**Parameters:**
- `must_gather_path` (string): Path to the must-gather directory

**Returns:**
- Hardware information including CPU topology, NUMA nodes, architecture, etc.

**Example:**
```python
await analyze_cluster_hardware(must_gather_path="/must-gather")
```

### 2. `validate_performance_requirements`

Validate if performance requirements are feasible given the cluster hardware.

**Parameters:**
- `workload_type` (string): Type of workload (e.g., "5g-ran", "database", "ai-inference")
- `isolated_cpu_count` (int, optional): Number of CPUs to isolate
- `reserved_cpu_count` (int, optional): Number of CPUs to reserve
- `enable_rt_kernel` (bool): Whether to enable real-time kernel
- `enable_dpdk` (bool): Whether to enable DPDK
- `hugepages_size` (string, optional): Hugepage size (e.g., "1G", "2M")
- `hugepages_count` (int, optional): Number of hugepages
- `power_mode` (string): Power mode ("default", "low-latency", "ultra-low-latency")

**Returns:**
- Validation results with errors, warnings, and recommendations

**Example:**
```python
await validate_performance_requirements(
    workload_type="5g-ran",
    reserved_cpu_count=8,
    enable_rt_kernel=True,
    power_mode="ultra-low-latency"
)
```

### 3. `generate_ppc_command`

Generate Performance Profile Creator command based on requirements.

**Parameters:**
- `workload_type` (string): Type of workload
- `mcp_name` (string): MachineConfigPool name (required)
- `profile_name` (string): Name of the profile (default: "performance")
- `reserved_cpu_count` (int, optional): Number of reserved CPUs
- `enable_rt_kernel` (bool): Enable RT kernel
- `disable_ht` (bool): Disable hyperthreading
- `enable_dpdk` (bool): Enable DPDK
- `power_mode` (string): Power consumption mode
- `topology_policy` (string): Topology manager policy
- `split_reserved_across_numa` (bool): Split reserved CPUs across NUMA nodes

**Returns:**
- Generated PPC command with explanation

**Example:**
```python
await generate_ppc_command(
    workload_type="5g-ran",
    mcp_name="worker-cnf",
    profile_name="5g-performance",
    enable_rt_kernel=True,
    power_mode="ultra-low-latency"
)
```

### 4. `create_performance_profile`

Execute the PPC command to create a PerformanceProfile YAML.

**Parameters:**
- `ppc_command` (string): The PPC command to execute
- `output_path` (string): Path to save the generated YAML (default: "/tmp/performance-profile.yaml")

**Returns:**
- Status and path to generated profile

**Example:**
```python
await create_performance_profile(
    ppc_command="podman run ...",
    output_path="/tmp/my-profile.yaml"
)
```

### 5. `list_workload_templates`

List available workload templates with their descriptions.

**Returns:**
- List of workload templates and their configurations

**Example:**
```python
await list_workload_templates()
```

### 6. `get_workload_recommendations`

Get workload recommendations based on natural language description.

**Parameters:**
- `workload_description` (string): Natural language description of the workload

**Returns:**
- Recommended workload type and configuration

**Example:**
```python
await get_workload_recommendations(
    workload_description="I need to run 5G RAN workloads with ultra-low latency and DPDK"
)
```

## Workload Templates

The server includes pre-configured templates for common workload types:

| Template | RT Kernel | DPDK | Power Mode | Use Cases |
|----------|-----------|------|------------|-----------|
| **5g-ran** | ✅ | ✅ | ultra-low-latency | 5G base stations, radio processing |
| **telco-vnf** | ✅ | ✅ | low-latency | Virtual routers, SBCs, packet gateways |
| **database** | ❌ | ❌ | default | PostgreSQL, MySQL, MongoDB |
| **ai-inference** | ❌ | ❌ | low-latency | TensorFlow, PyTorch inference |
| **hpc** | ❌ | ❌ | low-latency | Scientific computing, simulations |
| **low-latency-trading** | ✅ | ✅ | ultra-low-latency | HFT, market data processing |
| **media-processing** | ❌ | ❌ | default | Video transcoding, live streaming |

## Typical Workflow

### Step 1: Collect Must-Gather

First, collect must-gather data from your OpenShift cluster:

```bash
oc adm must-gather \
  --image=quay.io/openshift-kni/performance-addon-operator-must-gather:4.11 \
  --dest-dir=./must-gather
```

### Step 2: Analyze Hardware

Analyze your cluster's hardware topology:

```python
result = await analyze_cluster_hardware(must_gather_path="/must-gather")
print(result["summary"])
# Output:
# Cluster Hardware Summary:
# - Total Nodes: 3
# - Worker Nodes: 2
# - CPUs per Node: 48
# - NUMA Nodes per Node: 2
# - Hyperthreading: enabled
```

### Step 3: Get Recommendations

Describe your workload in natural language:

```python
result = await get_workload_recommendations(
    workload_description="I need to run 5G RAN workloads with real-time packet processing and DPDK"
)
print(result["recommendations"])
# Output suggests: workload_type="5g-ran", enable_rt_kernel=True, enable_dpdk=True
```

### Step 4: Validate Requirements

Validate that your requirements can be met:

```python
result = await validate_performance_requirements(
    workload_type="5g-ran",
    reserved_cpu_count=8,
    enable_rt_kernel=True,
    enable_dpdk=True,
    power_mode="ultra-low-latency"
)
print(result["validation"]["overall_status"])
# ✓ Configuration is valid and feasible
```

### Step 5: Generate PPC Command

Generate the optimized PPC command:

```python
result = await generate_ppc_command(
    workload_type="5g-ran",
    mcp_name="worker-cnf",
    profile_name="5g-performance"
)
print(result["command"])
print(result["explanation"])
```

### Step 6: Create Profile

Execute the command to create the PerformanceProfile YAML:

```python
result = await create_performance_profile(
    ppc_command=result["command"],
    output_path="/tmp/5g-performance-profile.yaml"
)
print(f"Profile created at: {result['output_path']}")
```

### Step 7: Apply to Cluster

Apply the generated profile to your cluster:

```bash
oc apply -f /tmp/5g-performance-profile.yaml
```

## Example: Using with AI Assistant

Here's how you might interact with this MCP server through an AI assistant (like Claude in Cursor):

```
You: "I need to set up a performance profile for 5G RAN workloads"

AI: Let me help you set that up. First, I'll analyze your cluster hardware.
    [Calls: analyze_cluster_hardware(must_gather_path="/must-gather")]
    
    Your cluster has 2 worker nodes, each with 48 CPUs and 2 NUMA nodes.
    
    For 5G RAN workloads, I recommend the "5g-ran" template which includes:
    - Real-time kernel
    - DPDK networking
    - Ultra-low-latency power mode
    
    Let me validate this configuration...
    [Calls: validate_performance_requirements(...)]
    
    ✓ Configuration is valid! I'll generate the PPC command for you.
    [Calls: generate_ppc_command(...)]
    
    Here's the command and what it does:
    [Shows explanation]
    
    Shall I create the profile?

You: "Yes, create it"

AI: [Calls: create_performance_profile(...)]
    Profile created at /tmp/5g-performance-profile.yaml
    
    To apply it to your cluster, run:
    oc apply -f /tmp/5g-performance-profile.yaml
```

## Advanced Usage

### Custom Workload Requirements

If none of the templates match your needs, you can specify custom requirements:

```python
# Get recommendations from natural language
recommendations = await get_workload_recommendations(
    workload_description="Database workload with some real-time requirements"
)

# Override specific settings
command = await generate_ppc_command(
    workload_type="custom",
    mcp_name="worker-db",
    reserved_cpu_count=6,
    enable_rt_kernel=False,
    power_mode="low-latency",
    topology_policy="restricted"
)
```

### Validating Before Generation

Always validate requirements before generating commands:

```python
# First validate
validation = await validate_performance_requirements(
    workload_type="telco-vnf",
    reserved_cpu_count=8,
    enable_rt_kernel=True
)

if validation["validation"]["is_valid"]:
    # Only generate if valid
    command = await generate_ppc_command(
        workload_type="telco-vnf",
        mcp_name="worker-cnf",
        reserved_cpu_count=8
    )
else:
    print("Errors:", validation["validation"]["errors"])
```

## Resources

The server provides MCP resources for direct access to data:

### `must-gather://hardware-info`

Access parsed hardware information as JSON:

```python
hardware_info = await fetch_resource("must-gather://hardware-info")
```

## Prompts

The server provides guided prompts for interactive workflows:

### `guided_profile_creation`

A step-by-step guided workflow for creating a performance profile:

```python
prompt = await get_prompt("guided_profile_creation")
```

## Troubleshooting

### Must-Gather Path Issues

If the parser fails to find nodes:

1. Verify must-gather was collected correctly:
   ```bash
   ls -la /path/to/must-gather/*/cluster-scoped-resources/core/nodes/
   ```

2. Ensure the directory is mounted correctly in the container:
   ```bash
   podman run -v /absolute/path:/must-gather:z ...
   ```

### PPC Command Execution Fails

If `create_performance_profile` fails:

1. Ensure podman is available in the container (it's included in the Containerfile)
2. Check that the must-gather path is accessible
3. Verify the PPC image is available:
   ```bash
   podman pull quay.io/openshift/origin-cluster-node-tuning-operator:4.11
   ```

### Validation Warnings

Validation warnings are informational and don't prevent profile creation:
- Review warnings carefully
- Adjust parameters if needed
- Warnings often suggest optimizations

## Development

### Running Locally (without container)

For development, you can run the server directly:

```bash
# Install dependencies
pip install -r requirements.txt

# Run the server
export MCP_TRANSPORT=stdio
python mcp_server.py
```

### Testing

Test individual components:

```python
# Test parser
from must_gather_parser import MustGatherParser
parser = MustGatherParser("/path/to/must-gather")
hardware = parser.analyze_hardware()
print(parser.get_summary())

# Test validator
from hardware_validator import HardwareValidator
validator = HardwareValidator(hardware)
result = validator.validate({
    "workload_type": "5g-ran",
    "reserved_cpu_count": 8,
    "enable_rt_kernel": True
})

# Test generator
from ppc_generator import PPCCommandGenerator
generator = PPCCommandGenerator(hardware)
params = generator.generate_parameters(
    workload_type="5g-ran",
    mcp_name="worker-cnf"
)
command = generator.build_command("/must-gather", params)
```

### Adding New Workload Templates

To add a new workload template, edit `workload_templates.py`:

```python
"my-workload": {
    "name": "My Custom Workload",
    "description": "Description of the workload",
    "default_config": {
        "enable_rt_kernel": False,
        "disable_ht": False,
        "enable_dpdk": False,
        "power_mode": "default",
        "topology_policy": "restricted",
        "split_reserved_across_numa": True,
        "per_pod_power_management": False
    },
    "recommended_hugepages": {
        "size": "2M",
        "note": "Why these hugepages"
    },
    "use_cases": [
        "Use case 1",
        "Use case 2"
    ]
}
```

## Contributing

Contributions are welcome! Areas for improvement:

1. **Enhanced Must-Gather Parsing**: Parse more detailed topology information
2. **Additional Templates**: Add more workload-specific templates
3. **Validation Rules**: Expand hardware validation checks
4. **Interactive UI**: Web interface for the MCP server
5. **Testing**: Add comprehensive test suite

## License

This project is part of the cluster-node-tuning-operator and follows the same license.

## References

- [OpenShift Node Tuning Operator](https://github.com/openshift/cluster-node-tuning-operator)
- [Performance Profile Documentation](../docs/performanceprofile/performance_profile.md)
- [Performance Profile Controller](../docs/performanceprofile/performance_controller.md)
- [Model Context Protocol (MCP)](https://modelcontextprotocol.io/)
- [FastMCP Documentation](https://github.com/jlowin/fastmcp)

## Support

For issues or questions:
1. Check the [troubleshooting section](#troubleshooting)
2. Review [PPC documentation](../cmd/performance-profile-creator/README.md)
3. Open an issue in the cluster-node-tuning-operator repository