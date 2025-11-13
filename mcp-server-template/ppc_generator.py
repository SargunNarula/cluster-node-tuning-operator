"""
PPC Command Generator

Generates Performance Profile Creator commands based on workload requirements
and hardware capabilities.
"""

from typing import Dict, Any, Optional


class PPCCommandGenerator:
    """Generates PPC commands based on requirements."""
    
    def __init__(self, hardware_info: Dict[str, Any]):
        """
        Initialize generator with hardware information.
        
        Args:
            hardware_info: Hardware information from MustGatherParser
        """
        self.hardware_info = hardware_info
        self.summary = hardware_info.get("summary", {})
        
    def generate_parameters(
        self,
        workload_type: str,
        mcp_name: str,
        profile_name: str = "performance",
        template: Optional[Dict[str, Any]] = None,
        **overrides
    ) -> Dict[str, Any]:
        """
        Generate PPC parameters based on workload type and overrides.
        
        Args:
            workload_type: Type of workload
            mcp_name: MachineConfigPool name
            profile_name: Profile name
            template: Workload template (if applicable)
            **overrides: User-specified overrides
            
        Returns:
            Dictionary of PPC parameters
        """
        # Start with template defaults or base defaults
        if template:
            params = template.get("default_config", {}).copy()
        else:
            params = self._get_base_defaults()
        
        # Always set these
        params["mcp_name"] = mcp_name
        params["profile_name"] = profile_name
        
        # Calculate CPU allocation if not provided
        if "reserved_cpu_count" not in overrides or overrides["reserved_cpu_count"] is None:
            if "reserved_cpu_count" not in params:
                params["reserved_cpu_count"] = self._calculate_reserved_cpus(workload_type)
        
        # Apply user overrides (only non-None values)
        for key, value in overrides.items():
            if value is not None:
                params[key] = value
        
        # Validate and adjust parameters
        params = self._adjust_parameters(params)
        
        return params
    
    def _get_base_defaults(self) -> Dict[str, Any]:
        """Get base default parameters."""
        return {
            "enable_rt_kernel": False,
            "disable_ht": False,
            "enable_dpdk": False,
            "power_mode": "default",
            "topology_policy": "restricted",
            "split_reserved_across_numa": False,
            "per_pod_power_management": False
        }
    
    def _calculate_reserved_cpus(self, workload_type: str) -> int:
        """
        Calculate appropriate number of reserved CPUs based on workload type.
        
        Args:
            workload_type: Type of workload
            
        Returns:
            Number of CPUs to reserve
        """
        cpus_per_node = self.summary.get("cpus_per_node", 0)
        numa_nodes = self.summary.get("numa_nodes_per_node", 1)
        
        if cpus_per_node == 0:
            return 4  # Safe default
        
        # Base calculation: 2-4 CPUs per NUMA node for system workloads
        base_reserved = max(2, numa_nodes * 2)
        
        # Adjust based on workload type
        if workload_type in ["5g-ran", "telco-vnf"]:
            # Ultra-low latency workloads: minimize reserved CPUs
            reserved = base_reserved
        elif workload_type == "database":
            # Database: balance between app and system
            reserved = max(4, int(cpus_per_node * 0.1))
        elif workload_type == "ai-inference":
            # AI workloads: leave more for application
            reserved = base_reserved
        else:
            # Default: conservative
            reserved = max(4, int(cpus_per_node * 0.1))
        
        # Ensure we don't reserve too many or too few
        reserved = max(2, min(reserved, cpus_per_node // 4))
        
        return reserved
    
    def _adjust_parameters(self, params: Dict[str, Any]) -> Dict[str, Any]:
        """
        Adjust parameters for consistency and hardware compatibility.
        
        Args:
            params: Parameter dictionary
            
        Returns:
            Adjusted parameters
        """
        # Handle mutually exclusive options
        if params.get("per_pod_power_management") and params.get("power_mode") != "default":
            # Per-pod PM and high power mode are mutually exclusive
            params["power_mode"] = "default"
        
        # Adjust topology policy for DPDK
        if params.get("enable_dpdk"):
            # DPDK typically benefits from single-numa-node
            if params.get("topology_policy") not in ["single-numa-node", "restricted"]:
                params["topology_policy"] = "single-numa-node"
        
        # Recommend split reserved across NUMA if multiple NUMA nodes
        numa_nodes = self.summary.get("numa_nodes_per_node", 1)
        if numa_nodes > 1 and not params.get("split_reserved_across_numa"):
            reserved = params.get("reserved_cpu_count", 0)
            if reserved >= numa_nodes * 2:
                # We have enough reserved CPUs to split
                params["split_reserved_across_numa"] = True
        
        return params
    
    def build_command(self, must_gather_path: str, params: Dict[str, Any]) -> str:
        """
        Build the PPC command string.
        
        Args:
            must_gather_path: Path to must-gather directory
            params: PPC parameters
            
        Returns:
            Complete PPC command string
        """
        # Determine image tag based on architecture
        architecture = self.summary.get("architectures", ["amd64"])[0]
        image_tag = "4.11"  # Default, should be configurable
        
        # Build command
        cmd_parts = [
            "podman run --entrypoint performance-profile-creator",
            f"-v {must_gather_path}:/must-gather:z",
            f"quay.io/openshift/origin-cluster-node-tuning-operator:{image_tag}",
            "--must-gather-dir-path /must-gather"
        ]
        
        # Add required parameters
        cmd_parts.append(f"--mcp-name {params['mcp_name']}")
        cmd_parts.append(f"--profile-name {params['profile_name']}")
        cmd_parts.append(f"--reserved-cpu-count {params['reserved_cpu_count']}")
        cmd_parts.append(f"--rt-kernel {str(params.get('enable_rt_kernel', False)).lower()}")
        
        # Add optional parameters
        if params.get("disable_ht"):
            cmd_parts.append("--disable-ht")
        
        if params.get("enable_dpdk"):
            cmd_parts.append("--user-level-networking")
        
        if params.get("split_reserved_across_numa"):
            cmd_parts.append("--split-reserved-cpus-across-numa")
        
        if params.get("per_pod_power_management"):
            cmd_parts.append("--per-pod-power-management")
        
        power_mode = params.get("power_mode", "default")
        if power_mode != "default":
            cmd_parts.append(f"--power-consumption-mode {power_mode}")
        
        topology_policy = params.get("topology_policy", "restricted")
        cmd_parts.append(f"--topology-manager-policy {topology_policy}")
        
        # Add output redirection
        cmd_parts.append("> performance-profile.yaml")
        
        return " \\\n  ".join(cmd_parts)
    
    def explain_parameters(self, params: Dict[str, Any]) -> str:
        """
        Generate human-readable explanation of parameters.
        
        Args:
            params: PPC parameters
            
        Returns:
            Explanation string
        """
        explanations = []
        
        explanations.append("Parameter Explanation:")
        explanations.append("=" * 50)
        
        # MCP and profile
        explanations.append(f"\n📦 Profile Configuration:")
        explanations.append(f"  - Profile Name: {params.get('profile_name', 'performance')}")
        explanations.append(f"  - MachineConfigPool: {params['mcp_name']}")
        explanations.append(f"    → This profile will apply to nodes in the '{params['mcp_name']}' MCP")
        
        # CPU allocation
        explanations.append(f"\n🖥️  CPU Allocation:")
        reserved = params.get("reserved_cpu_count", 0)
        cpus_per_node = self.summary.get("cpus_per_node", 0)
        isolated = cpus_per_node - reserved if cpus_per_node > 0 else "calculated"
        
        explanations.append(f"  - Reserved CPUs: {reserved}")
        explanations.append(f"    → Used for system processes (kubelet, container runtime)")
        explanations.append(f"  - Isolated CPUs: ~{isolated}")
        explanations.append(f"    → Dedicated to application workloads")
        
        if params.get("disable_ht"):
            explanations.append(f"  - Hyperthreading: DISABLED")
            explanations.append(f"    → Reduces CPU count but improves deterministic behavior")
        
        if params.get("split_reserved_across_numa"):
            numa = self.summary.get("numa_nodes_per_node", 1)
            explanations.append(f"  - Reserved CPUs will be split across {numa} NUMA node(s)")
            explanations.append(f"    → Improves NUMA locality for system processes")
        
        # Kernel
        explanations.append(f"\n⚙️  Kernel Configuration:")
        if params.get("enable_rt_kernel"):
            explanations.append(f"  - Real-Time Kernel: ENABLED")
            explanations.append(f"    → Provides deterministic latency for time-sensitive workloads")
            explanations.append(f"    ⚠️  Requires node reboot during application")
        else:
            explanations.append(f"  - Real-Time Kernel: DISABLED")
            explanations.append(f"    → Standard kernel will be used")
        
        # Network
        if params.get("enable_dpdk"):
            explanations.append(f"\n🌐 Network Configuration:")
            explanations.append(f"  - User-Level Networking (DPDK): ENABLED")
            explanations.append(f"    → Optimizes for high-throughput packet processing")
            explanations.append(f"    → Typically requires 1G hugepages")
        
        # Topology
        explanations.append(f"\n🗺️  NUMA Topology:")
        policy = params.get("topology_policy", "restricted")
        explanations.append(f"  - Topology Manager Policy: {policy}")
        if policy == "single-numa-node":
            explanations.append(f"    → Pods must fit within a single NUMA node")
        elif policy == "restricted":
            explanations.append(f"    → Pods prefer single NUMA node but can span")
        else:
            explanations.append(f"    → Best-effort NUMA alignment")
        
        # Power
        explanations.append(f"\n⚡ Power Management:")
        power_mode = params.get("power_mode", "default")
        explanations.append(f"  - Power Mode: {power_mode}")
        
        if power_mode == "ultra-low-latency":
            explanations.append(f"    → Maximum performance, highest power consumption")
            explanations.append(f"    → C-states disabled, CPU frequency maximized")
        elif power_mode == "low-latency":
            explanations.append(f"    → Balanced low-latency with controlled power")
        else:
            explanations.append(f"    → Standard power management")
        
        if params.get("per_pod_power_management"):
            explanations.append(f"  - Per-Pod Power Management: ENABLED")
            explanations.append(f"    → Allows pod-level power tuning")
        
        return "\n".join(explanations)


