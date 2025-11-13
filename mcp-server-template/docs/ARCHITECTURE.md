# Architecture Overview

## System Design

The Performance Profile Creator MCP Server is an intelligent wrapper around the OpenShift Performance Profile Creator (PPC) tool. It bridges the gap between natural language workload requirements and the deterministic PPC command-line tool.

## Components

```
┌─────────────────────────────────────────────────────────────────┐
│                        MCP CLIENT                                │
│              (Cursor, Claude Desktop, etc.)                      │
└───────────────────────────┬─────────────────────────────────────┘
                            │ MCP Protocol (JSON-RPC over stdio)
                            ▼
┌─────────────────────────────────────────────────────────────────┐
│                    MCP SERVER (mcp_server.py)                    │
├─────────────────────────────────────────────────────────────────┤
│  Tools:                                                          │
│  • analyze_cluster_hardware()      - Parse must-gather          │
│  • validate_performance_requirements() - Check feasibility       │
│  • generate_ppc_command()          - Create PPC command          │
│  • create_performance_profile()    - Execute PPC                 │
│  • list_workload_templates()       - List templates              │
│  • get_workload_recommendations()  - NL understanding            │
│                                                                   │
│  Resources:                                                       │
│  • must-gather://hardware-info     - Hardware data               │
│                                                                   │
│  Prompts:                                                         │
│  • guided_profile_creation         - Step-by-step workflow       │
└───────┬─────────┬──────────┬──────────┬──────────────────────────┘
        │         │          │          │
        ▼         ▼          ▼          ▼
┌──────────┐ ┌─────────┐ ┌─────┐ ┌──────────┐
│Must-     │ │Hardware │ │PPC  │ │Workload  │
│Gather    │ │Validator│ │Gen  │ │Templates │
│Parser    │ │         │ │     │ │          │
└──────────┘ └─────────┘ └─────┘ └──────────┘
     │                       │
     ▼                       ▼
┌──────────┐          ┌───────────┐
│Must-     │          │PPC Tool   │
│Gather    │          │(podman)   │
│Data      │          │           │
└──────────┘          └───────────┘
```

## Module Descriptions

### 1. mcp_server.py
**Main MCP Server Implementation**

- Implements MCP protocol using FastMCP
- Exposes tools, resources, and prompts
- Coordinates between modules
- Maintains global state for hardware data

**Key Responsibilities:**
- MCP protocol handling
- State management
- Tool orchestration
- Error handling and user feedback

### 2. must_gather_parser.py
**Hardware Topology Extractor**

Parses OpenShift must-gather data to extract cluster hardware information.

**What it parses:**
- Node information (names, roles, labels)
- CPU topology (cores, sockets, threads)
- NUMA configuration
- Memory capacity
- Architecture (x86_64, aarch64)
- Kernel version

**Output:**
```python
{
  "nodes": [...],
  "summary": {
    "total_nodes": 3,
    "worker_nodes": 2,
    "cpus_per_node": 48,
    "numa_nodes_per_node": 2,
    "hyperthreading_enabled": true,
    "architectures": ["amd64"]
  }
}
```

**Fallback Strategy:**
If must-gather parsing fails, it provides reasonable defaults for demonstration purposes.

### 3. hardware_validator.py
**Feasibility Checker**

Validates performance requirements against actual hardware capabilities.

**Validation Checks:**
- **CPU Allocation**: Ensures reserved + isolated ≤ available CPUs
- **NUMA Alignment**: Checks if CPU distribution aligns with NUMA topology
- **RT Kernel**: Validates architecture compatibility
- **Hugepages**: Checks size validity for architecture
- **Memory**: Estimates memory requirements for hugepages
- **Power Mode**: Warns about power/performance tradeoffs

**Output:**
```python
{
  "is_valid": true,
  "errors": [],
  "warnings": ["..."],
  "recommendations": ["..."],
  "detailed_checks": {
    "cpu": {...},
    "rt_kernel": {...},
    "hugepages": {...}
  }
}
```

### 4. ppc_generator.py
**PPC Command Builder**

Generates optimized PPC commands based on workload requirements and hardware.

**Process:**
1. Start with workload template (if applicable)
2. Calculate CPU allocation (if not specified)
3. Apply user overrides
4. Adjust for consistency (e.g., mutual exclusions)
5. Build command string
6. Generate human-readable explanation

**Intelligent Features:**
- Auto-calculates reserved CPUs based on workload type
- Adjusts topology policy for DPDK workloads
- Recommends NUMA splitting when beneficial
- Handles mutually exclusive options

### 5. workload_templates.py
**Predefined Configurations**

Provides templates for common workload types with battle-tested configurations.

**Available Templates:**
- **5g-ran**: Ultra-low latency, RT kernel, DPDK, HT disabled
- **telco-vnf**: RT kernel, DPDK, NUMA-aware
- **database**: Memory optimized, no RT kernel
- **ai-inference**: CPU isolated, low latency
- **hpc**: Compute optimized, NUMA-aware
- **low-latency-trading**: Ultra-low latency, RT kernel
- **media-processing**: Balanced, per-pod power management

**Template Structure:**
```python
{
  "name": "Human-readable name",
  "description": "Detailed description",
  "default_config": {
    "enable_rt_kernel": bool,
    "power_mode": str,
    # ... other settings
  },
  "recommended_hugepages": {...},
  "use_cases": [...]
}
```

## Data Flow

### Typical Workflow

```
1. User provides natural language requirement
   ↓
2. MCP Server calls get_workload_recommendations()
   ↓
3. Parses description, identifies keywords
   ↓
4. Returns recommended workload_type and settings
   ↓
5. User (or AI) calls analyze_cluster_hardware()
   ↓
6. MustGatherParser extracts hardware topology
   ↓
7. Hardware data stored in global state
   ↓
8. User calls validate_performance_requirements()
   ↓
9. HardwareValidator checks feasibility
   ↓
10. Returns validation results with warnings/recommendations
   ↓
11. User calls generate_ppc_command()
   ↓
12. PPCCommandGenerator builds command
   ↓
13. Returns command with explanation
   ↓
14. User calls create_performance_profile()
   ↓
15. Server executes PPC via podman
   ↓
16. Returns PerformanceProfile YAML
   ↓
17. User applies YAML to cluster with oc apply
```

## Key Design Decisions

### 1. Stateful Global Storage
**Decision:** Store hardware data in global `_must_gather_data` variable

**Rationale:**
- Avoid re-parsing must-gather for every operation
- Simplify tool interfaces
- Enable chaining of operations

**Trade-off:** Single-user assumption (acceptable for MCP stdio transport)

### 2. Template-Based Configuration
**Decision:** Provide predefined templates rather than pure NL generation

**Rationale:**
- Ensures battle-tested configurations
- Reduces risk of incorrect settings
- Provides educational value (users learn from templates)
- Allows overrides for customization

**Trade-off:** Less flexibility, but much safer

### 3. Validation Before Generation
**Decision:** Separate validation tool from command generation

**Rationale:**
- Early feedback to users
- Prevents generating invalid configurations
- Educational (explains why something won't work)
- Allows iterative refinement

### 4. Human-Readable Explanations
**Decision:** Generate detailed explanations for all parameters

**Rationale:**
- Educates users about performance tuning
- Builds trust in automated decisions
- Enables informed overrides
- Reduces "magic" factor

### 5. Podman Integration
**Decision:** Execute PPC directly rather than just generating commands

**Rationale:**
- Complete end-to-end workflow
- Immediate validation of command correctness
- Returns ready-to-apply YAML
- Better UX

**Trade-off:** Requires podman in container (added to Containerfile)

## Extension Points

### Adding New Workload Templates
1. Edit `workload_templates.py`
2. Add entry to `TEMPLATES` dict
3. Define `default_config`, `description`, `use_cases`
4. Template automatically available via `list_workload_templates()`

### Enhancing Must-Gather Parsing
1. Edit `must_gather_parser.py`
2. Add parsing logic in `_extract_node_info()` or `_extract_cpu_topology()`
3. Update return structure if needed
4. Parser automatically used by all tools

### Adding Validation Rules
1. Edit `hardware_validator.py`
2. Add new method `_validate_xxx()`
3. Call from `validate()` method
4. Return structured validation results

### Adding PPC Parameters
1. Edit `ppc_generator.py`
2. Add parameter to `generate_parameters()`
3. Add command-line flag in `build_command()`
4. Add explanation in `explain_parameters()`

## Security Considerations

1. **Command Injection**: PPC command uses fixed structure with parameterized values
2. **Path Traversal**: Must-gather path validated before use
3. **Resource Limits**: subprocess timeout prevents hanging
4. **Container Isolation**: Runs in rootless podman with limited permissions

## Performance Considerations

1. **Must-Gather Parsing**: Cached after first parse
2. **File I/O**: Lazy loading, only parse what's needed
3. **Validation**: Fast checks first, expensive checks last
4. **Memory**: Bounded by must-gather size (typically < 100MB)

## Future Enhancements

1. **Enhanced Parsing**: Use Performance Addon Operator's own must-gather format
2. **Live Cluster**: Query live cluster APIs instead of must-gather
3. **Profile Validation**: Validate generated YAML against OpenAPI schema
4. **Historical Data**: Track performance of generated profiles
5. **ML Recommendations**: Learn from successful configurations
6. **Web UI**: Interactive visual configuration builder
7. **Multi-Node**: Support heterogeneous clusters
8. **Testing**: Comprehensive test suite with fixtures

## Testing Strategy

### Unit Tests
- Test each module independently
- Mock must-gather data
- Verify parameter calculations
- Check validation logic

### Integration Tests
- Test tool chaining
- Verify end-to-end workflow
- Test with real must-gather data

### MCP Protocol Tests
- Verify JSON-RPC compliance
- Test tool registration
- Test resource access
- Test prompt rendering

## Dependencies

### Runtime Dependencies
- `fastmcp`: MCP protocol implementation
- `pyyaml`: Must-gather YAML parsing
- `podman`: PPC execution

### Optional Dependencies
- `httpx`: Future API integrations
- `pytest`: Testing
- `mypy`: Type checking

## Deployment

### Container-Based (Recommended)
```bash
podman build -t ppc-mcp:latest .
podman run -i --rm -v /must-gather:/must-gather:z ppc-mcp:latest
```

### Local Development
```bash
pip install -r requirements.txt
python mcp_server.py
```

### MCP Client Configuration
```json
{
  "mcpServers": {
    "performance-profile-creator": {
      "command": "podman",
      "args": ["run", "-i", "--rm", ...],
      "env": {"MCP_TRANSPORT": "stdio"}
    }
  }
}
```

## Monitoring and Debugging

### Logging
- MCP protocol messages visible in client logs
- Python exceptions returned as MCP errors
- Use `print()` for debugging (goes to stderr)

### Common Issues
1. **Must-gather not found**: Check volume mount
2. **PPC image pull fails**: Pre-pull image
3. **Validation failures**: Review warnings, adjust parameters
4. **Podman not available**: Ensure Containerfile installs podman

## Contributing Guidelines

1. **Code Style**: Follow PEP 8
2. **Type Hints**: Use type hints for all functions
3. **Documentation**: Docstrings for all public methods
4. **Testing**: Add tests for new features
5. **Backwards Compatibility**: Don't break existing tool interfaces

## License

Part of cluster-node-tuning-operator project, same license applies.




