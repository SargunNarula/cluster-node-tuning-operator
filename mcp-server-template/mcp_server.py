"""
Performance Profile Creator MCP Server

An intelligent wrapper for the OpenShift Performance Profile Creator (PPC) tool
that understands workload requirements in natural language, validates against
hardware capabilities from must-gather data, and generates optimized PPC commands.
"""

import os
import json
import subprocess
from typing import Any, Optional, List, Tuple
from pathlib import Path

from mcp.server.fastmcp import FastMCP
from must_gather_parser import MustGatherParser
from hardware_validator import HardwareValidator
from ppc_generator import PPCCommandGenerator
from workload_templates import WorkloadTemplates

# Initialize MCP server
mcp = FastMCP("performance-profile-creator")

# Global state for must-gather data
_must_gather_data: Optional[dict] = None
_must_gather_path: Optional[str] = None

# Path mapping configuration
_path_mappings: List[Tuple[str, str]] = []


def _init_path_mappings():
    """
    Initialize path mappings from environment variables.
    
    Supports multiple path mapping formats:
    1. MCP_PATH_MAPPINGS: JSON array of [host_path, container_path] pairs
    2. MCP_PATH_MAP_<N>: Individual mappings as "host_path:container_path"
    3. Default mapping for common paths if running in container
    """
    global _path_mappings
    
    # Check for running in container
    in_container = os.path.exists('/.dockerenv') or os.path.exists('/run/.containerenv')
    
    # Method 1: JSON array format
    mappings_json = os.environ.get('MCP_PATH_MAPPINGS')
    if mappings_json:
        try:
            mappings = json.loads(mappings_json)
            for mapping in mappings:
                if len(mapping) == 2:
                    _path_mappings.append((mapping[0], mapping[1]))
                    print(f"[Path Mapping] {mapping[0]} -> {mapping[1]}")
        except json.JSONDecodeError as e:
            print(f"Warning: Failed to parse MCP_PATH_MAPPINGS: {e}")
    
    # Method 2: Individual environment variables
    i = 0
    while True:
        mapping = os.environ.get(f'MCP_PATH_MAP_{i}')
        if not mapping:
            break
        try:
            host_path, container_path = mapping.split(':', 1)
            _path_mappings.append((host_path, container_path))
            print(f"[Path Mapping] {host_path} -> {container_path}")
        except ValueError:
            print(f"Warning: Invalid path mapping format for MCP_PATH_MAP_{i}: {mapping}")
        i += 1
    
    # Method 3: Auto-detect common container mount paths
    if in_container and not _path_mappings:
        # Check for common mount points
        common_mounts = [
            ('/must-gather', None),  # Will be auto-detected
            ('/data', None),
            ('/workspace', None)
        ]
        
        for container_path, _ in common_mounts:
            if os.path.exists(container_path):
                print(f"[Path Mapping] Auto-detected container mount: {container_path}")
                # We can't know the host path, but we know the container path exists
                # Store as empty host path to indicate "use container path as-is"
                _path_mappings.append(('', container_path))
    
    if _path_mappings:
        print(f"[Path Mapping] Initialized {len(_path_mappings)} path mapping(s)")
    elif in_container:
        print("[Path Mapping] No explicit mappings configured. Will attempt auto-resolution.")


def resolve_path(input_path: str) -> str:
    """
    Resolve a path that might be a host path to its container equivalent.
    
    Args:
        input_path: Input path (could be host or container path)
        
    Returns:
        Resolved container path
    """
    global _path_mappings
    
    # If no mappings configured, return as-is
    if not _path_mappings:
        return input_path
    
    input_path_obj = Path(input_path).resolve()
    input_path_str = str(input_path_obj)
    
    # Try each mapping
    for host_path, container_path in _path_mappings:
        if not host_path:
            # Empty host path means use container path if it matches
            if input_path_str.startswith(container_path):
                print(f"[Path Resolution] Path already in container format: {input_path} -> {input_path}")
                return input_path
            continue
        
        # Check if input path starts with host path
        host_path_obj = Path(host_path).resolve()
        try:
            # Check if input_path is relative to host_path
            rel_path = input_path_obj.relative_to(host_path_obj)
            # Construct container path
            resolved = str(Path(container_path) / rel_path)
            print(f"[Path Resolution] Resolved: {input_path} -> {resolved}")
            return resolved
        except ValueError:
            # Not relative to this host path, try next mapping
            continue
    
    # No mapping found - check if path exists as-is (might already be container path)
    if os.path.exists(input_path):
        print(f"[Path Resolution] Path exists as-is: {input_path}")
        return input_path
    
    # No resolution found, return original
    print(f"[Path Resolution] No mapping found for: {input_path}, using as-is")
    return input_path


# Initialize path mappings on module load
_init_path_mappings()


@mcp.tool()
async def analyze_cluster_hardware(must_gather_path: str) -> dict[str, Any]:
    """
    Analyze cluster hardware topology from must-gather data.
    
    Args:
        must_gather_path: Path to the must-gather directory (can be host or container path)
        
    Returns:
        Dictionary containing cluster hardware information including:
        - Number of nodes
        - CPU topology (cores, NUMA nodes, hyperthreading)
        - Memory information
        - Network devices
        - Architecture (x86_64, aarch64)
        
    Note:
        When running in a container, this tool automatically resolves host paths
        to their container-mounted equivalents based on configured path mappings.
    """
    global _must_gather_data, _must_gather_path
    
    try:
        # Resolve path (host path -> container path if needed)
        resolved_path = resolve_path(must_gather_path)
        
        parser = MustGatherParser(resolved_path)
        hardware_info = parser.analyze_hardware()
        
        # Store for later use
        _must_gather_data = hardware_info
        _must_gather_path = resolved_path
        
        return {
            "success": True,
            "hardware_info": hardware_info,
            "summary": parser.get_summary(),
            "path_info": {
                "original_path": must_gather_path,
                "resolved_path": resolved_path
            }
        }
    except Exception as e:
        return {
            "success": False,
            "error": str(e),
            "message": "Failed to analyze must-gather data. Please ensure the path is correct.",
            "path_info": {
                "original_path": must_gather_path,
                "resolved_path": resolve_path(must_gather_path) if must_gather_path else None
            }
        }


@mcp.tool()
async def validate_performance_requirements(
    workload_type: str,
    isolated_cpu_count: Optional[int] = None,
    reserved_cpu_count: Optional[int] = None,
    enable_rt_kernel: bool = False,
    enable_dpdk: bool = False,
    hugepages_size: Optional[str] = None,
    hugepages_count: Optional[int] = None,
    power_mode: str = "default"
) -> dict[str, Any]:
    """
    Validate if performance requirements are feasible given the cluster hardware.
    
    Args:
        workload_type: Type of workload (e.g., "5g-ran", "database", "ai-inference", "telco-vnf", "custom")
        isolated_cpu_count: Number of CPUs to isolate for application workloads
        reserved_cpu_count: Number of CPUs to reserve for system/kubelet
        enable_rt_kernel: Whether to enable real-time kernel
        enable_dpdk: Whether to enable user-level networking (DPDK)
        hugepages_size: Hugepage size (e.g., "1G", "2M")
        hugepages_count: Number of hugepages
        power_mode: Power consumption mode ("default", "low-latency", "ultra-low-latency")
        
    Returns:
        Validation results with warnings and recommendations
    """
    global _must_gather_data
    
    if not _must_gather_data:
        return {
            "success": False,
            "error": "No hardware data available. Please run analyze_cluster_hardware first."
        }
    
    try:
        validator = HardwareValidator(_must_gather_data)
        
        requirements = {
            "workload_type": workload_type,
            "isolated_cpu_count": isolated_cpu_count,
            "reserved_cpu_count": reserved_cpu_count,
            "enable_rt_kernel": enable_rt_kernel,
            "enable_dpdk": enable_dpdk,
            "hugepages_size": hugepages_size,
            "hugepages_count": hugepages_count,
            "power_mode": power_mode
        }
        
        validation_result = validator.validate(requirements)
        
        return {
            "success": True,
            "validation": validation_result
        }
    except Exception as e:
        return {
            "success": False,
            "error": str(e)
        }


@mcp.tool()
async def generate_ppc_command(
    workload_type: str,
    mcp_name: str,
    profile_name: str = "performance",
    isolated_cpu_count: Optional[int] = None,
    reserved_cpu_count: Optional[int] = None,
    enable_rt_kernel: bool = False,
    disable_ht: bool = False,
    enable_dpdk: bool = False,
    power_mode: str = "default",
    topology_policy: str = "restricted",
    split_reserved_across_numa: bool = False,
    per_pod_power_management: bool = False
) -> dict[str, Any]:
    """
    Generate Performance Profile Creator command based on requirements.
    
    Args:
        workload_type: Type of workload - used for template-based generation
        mcp_name: MachineConfigPool name (required, e.g., "worker-cnf")
        profile_name: Name of the performance profile (default: "performance")
        isolated_cpu_count: Number of CPUs to isolate (if None, calculated from template)
        reserved_cpu_count: Number of CPUs to reserve (required for custom workloads)
        enable_rt_kernel: Enable real-time kernel
        disable_ht: Disable hyperthreading
        enable_dpdk: Enable user-level networking (DPDK)
        power_mode: Power consumption mode ("default", "low-latency", "ultra-low-latency")
        topology_policy: Topology manager policy ("single-numa-node", "best-effort", "restricted")
        split_reserved_across_numa: Split reserved CPUs across NUMA nodes
        per_pod_power_management: Enable per-pod power management
        
    Returns:
        Generated PPC command with explanation
    """
    global _must_gather_data, _must_gather_path
    
    if not _must_gather_data or not _must_gather_path:
        return {
            "success": False,
            "error": "No hardware data available. Please run analyze_cluster_hardware first."
        }
    
    try:
        generator = PPCCommandGenerator(_must_gather_data)
        
        # Get workload template if applicable
        template = WorkloadTemplates.get_template(workload_type)
        
        # Merge template with user overrides
        params = generator.generate_parameters(
            workload_type=workload_type,
            mcp_name=mcp_name,
            profile_name=profile_name,
            template=template,
            isolated_cpu_count=isolated_cpu_count,
            reserved_cpu_count=reserved_cpu_count,
            enable_rt_kernel=enable_rt_kernel,
            disable_ht=disable_ht,
            enable_dpdk=enable_dpdk,
            power_mode=power_mode,
            topology_policy=topology_policy,
            split_reserved_across_numa=split_reserved_across_numa,
            per_pod_power_management=per_pod_power_management
        )
        
        command = generator.build_command(_must_gather_path, params)
        
        return {
            "success": True,
            "command": command,
            "parameters": params,
            "explanation": generator.explain_parameters(params),
            "workload_template": template.get("name") if template else None
        }
    except Exception as e:
        return {
            "success": False,
            "error": str(e)
        }


@mcp.tool()
async def create_performance_profile(
    ppc_command: str,
    output_path: str = "/tmp/performance-profile.yaml"
) -> dict[str, Any]:
    """
    Execute the PPC command to create a PerformanceProfile YAML.
    
    Args:
        ppc_command: The PPC command to execute (from generate_ppc_command)
        output_path: Path where to save the generated YAML (default: /tmp/performance-profile.yaml)
        
    Returns:
        Status and path to generated profile
    """
    try:
        # Execute the PPC command
        result = subprocess.run(
            ppc_command,
            shell=True,
            capture_output=True,
            text=True,
            timeout=60
        )
        
        if result.returncode != 0:
            return {
                "success": False,
                "error": f"PPC command failed: {result.stderr}"
            }
        
        # Save the output
        with open(output_path, 'w') as f:
            f.write(result.stdout)
        
        return {
            "success": True,
            "output_path": output_path,
            "profile_yaml": result.stdout,
            "message": f"Performance profile created successfully at {output_path}"
        }
    except subprocess.TimeoutExpired:
        return {
            "success": False,
            "error": "PPC command timed out after 60 seconds"
        }
    except Exception as e:
        return {
            "success": False,
            "error": str(e)
        }


@mcp.tool()
async def list_workload_templates() -> dict[str, Any]:
    """
    List available workload templates with their descriptions.
    
    Returns:
        List of workload templates and their configurations
    """
    templates = WorkloadTemplates.list_templates()
    return {
        "success": True,
        "templates": templates
    }


@mcp.tool()
async def get_workload_recommendations(
    workload_description: str
) -> dict[str, Any]:
    """
    Get workload recommendations based on natural language description.
    
    This tool uses heuristics to recommend appropriate settings based on
    keywords in the workload description.
    
    Args:
        workload_description: Natural language description of the workload
        
    Returns:
        Recommended workload type and configuration
    """
    description_lower = workload_description.lower()
    
    recommendations = {
        "workload_type": "custom",
        "recommended_settings": {},
        "reasoning": []
    }
    
    # Analyze keywords for workload type
    if any(kw in description_lower for kw in ["5g", "ran", "radio", "baseband"]):
        recommendations["workload_type"] = "5g-ran"
        recommendations["reasoning"].append("Detected 5G/RAN workload - requires ultra-low latency")
    elif any(kw in description_lower for kw in ["database", "db", "sql", "postgres", "mysql"]):
        recommendations["workload_type"] = "database"
        recommendations["reasoning"].append("Detected database workload - requires memory optimization")
    elif any(kw in description_lower for kw in ["ai", "ml", "inference", "tensorflow", "pytorch"]):
        recommendations["workload_type"] = "ai-inference"
        recommendations["reasoning"].append("Detected AI/ML workload - requires CPU isolation")
    elif any(kw in description_lower for kw in ["vnf", "telco", "nfv"]):
        recommendations["workload_type"] = "telco-vnf"
        recommendations["reasoning"].append("Detected Telco VNF workload - requires RT kernel and DPDK")
    
    # Detect specific requirements
    if any(kw in description_lower for kw in ["real-time", "realtime", "rt", "low latency", "latency-sensitive"]):
        recommendations["recommended_settings"]["enable_rt_kernel"] = True
        recommendations["recommended_settings"]["power_mode"] = "ultra-low-latency"
        recommendations["reasoning"].append("Real-time/low-latency keywords detected - enabling RT kernel")
    
    if any(kw in description_lower for kw in ["dpdk", "user space networking", "packet processing"]):
        recommendations["recommended_settings"]["enable_dpdk"] = True
        recommendations["recommended_settings"]["topology_policy"] = "single-numa-node"
        recommendations["reasoning"].append("DPDK/packet processing detected - enabling user-level networking")
    
    if any(kw in description_lower for kw in ["power efficient", "power saving", "energy"]):
        recommendations["recommended_settings"]["per_pod_power_management"] = True
        recommendations["recommended_settings"]["power_mode"] = "default"
        recommendations["reasoning"].append("Power efficiency mentioned - enabling per-pod power management")
    
    if any(kw in description_lower for kw in ["numa", "locality", "affinity"]):
        recommendations["recommended_settings"]["topology_policy"] = "single-numa-node"
        recommendations["reasoning"].append("NUMA awareness mentioned - setting single-numa-node policy")
    
    # Get template if identified
    template = WorkloadTemplates.get_template(recommendations["workload_type"])
    if template:
        recommendations["template_details"] = template
    
    return {
        "success": True,
        "recommendations": recommendations
    }


@mcp.resource("must-gather://hardware-info")
async def get_hardware_info() -> str:
    """Resource providing current cluster hardware information."""
    global _must_gather_data
    
    if not _must_gather_data:
        return json.dumps({
            "error": "No hardware data available. Run analyze_cluster_hardware first."
        }, indent=2)
    
    return json.dumps(_must_gather_data, indent=2)


@mcp.prompt()
async def guided_profile_creation() -> str:
    """
    Guided workflow for creating a performance profile.
    
    This prompt helps users step-by-step through the process of:
    1. Analyzing cluster hardware
    2. Understanding workload requirements
    3. Validating feasibility
    4. Generating the profile
    """
    return """I'll help you create an optimized Performance Profile for your OpenShift cluster.

Let's go through this step by step:

1. **Analyze Cluster Hardware**
   First, I need to analyze your cluster's hardware topology.
   - Do you have a must-gather directory? If so, provide the path.
   - Use: analyze_cluster_hardware(must_gather_path="/path/to/must-gather")

2. **Understand Your Workload**
   Tell me about your workload in natural language, for example:
   - "I need to run 5G RAN workloads with ultra-low latency"
   - "Database server requiring memory optimization"
   - "AI inference with CPU isolation"
   - Use: get_workload_recommendations(workload_description="your description")

3. **Validate Requirements**
   Based on your workload, I'll validate if your hardware can support it.
   - Use: validate_performance_requirements(workload_type="...", ...)

4. **Generate PPC Command**
   I'll generate the optimal PPC command for your setup.
   - Use: generate_ppc_command(workload_type="...", mcp_name="worker-cnf", ...)

5. **Create the Profile**
   Finally, execute the PPC command to create your PerformanceProfile YAML.
   - Use: create_performance_profile(ppc_command="...", output_path="...")

Ready to start? Let me know your must-gather path!"""


if __name__ == "__main__":
    mcp.run(transport=os.environ.get("MCP_TRANSPORT", "stdio"))
