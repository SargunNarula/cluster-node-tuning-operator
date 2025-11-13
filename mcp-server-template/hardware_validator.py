"""
Hardware Validator

Validates performance profile requirements against cluster hardware capabilities.
"""

from typing import Dict, Any, List, Optional


class HardwareValidator:
    """Validates performance requirements against hardware capabilities."""
    
    def __init__(self, hardware_info: Dict[str, Any]):
        """
        Initialize validator with hardware information.
        
        Args:
            hardware_info: Hardware information from MustGatherParser
        """
        self.hardware_info = hardware_info
        self.summary = hardware_info.get("summary", {})
        
    def validate(self, requirements: Dict[str, Any]) -> Dict[str, Any]:
        """
        Validate requirements against hardware.
        
        Args:
            requirements: Dictionary of performance requirements
            
        Returns:
            Validation results with status, warnings, and recommendations
        """
        validation = {
            "is_valid": True,
            "errors": [],
            "warnings": [],
            "recommendations": [],
            "detailed_checks": {}
        }
        
        # Validate CPU requirements
        cpu_validation = self._validate_cpu(requirements)
        validation["detailed_checks"]["cpu"] = cpu_validation
        if not cpu_validation["is_valid"]:
            validation["is_valid"] = False
            validation["errors"].extend(cpu_validation["errors"])
        validation["warnings"].extend(cpu_validation.get("warnings", []))
        validation["recommendations"].extend(cpu_validation.get("recommendations", []))
        
        # Validate RT kernel compatibility
        rt_validation = self._validate_rt_kernel(requirements)
        validation["detailed_checks"]["rt_kernel"] = rt_validation
        validation["warnings"].extend(rt_validation.get("warnings", []))
        validation["recommendations"].extend(rt_validation.get("recommendations", []))
        
        # Validate hugepages
        hugepages_validation = self._validate_hugepages(requirements)
        validation["detailed_checks"]["hugepages"] = hugepages_validation
        if not hugepages_validation["is_valid"]:
            validation["warnings"].extend(hugepages_validation["errors"])
        validation["recommendations"].extend(hugepages_validation.get("recommendations", []))
        
        # Validate power mode
        power_validation = self._validate_power_mode(requirements)
        validation["detailed_checks"]["power_mode"] = power_validation
        validation["warnings"].extend(power_validation.get("warnings", []))
        
        # Generate overall recommendation
        if validation["is_valid"]:
            validation["overall_status"] = "✓ Configuration is valid and feasible"
        else:
            validation["overall_status"] = "✗ Configuration has issues that must be addressed"
        
        return validation
    
    def _validate_cpu(self, requirements: Dict[str, Any]) -> Dict[str, Any]:
        """Validate CPU-related requirements."""
        result = {
            "is_valid": True,
            "errors": [],
            "warnings": [],
            "recommendations": []
        }
        
        cpus_per_node = self.summary.get("cpus_per_node", 0)
        numa_nodes = self.summary.get("numa_nodes_per_node", 1)
        ht_enabled = self.summary.get("hyperthreading_enabled", True)
        
        isolated = requirements.get("isolated_cpu_count")
        reserved = requirements.get("reserved_cpu_count")
        
        if reserved is None:
            result["errors"].append("Reserved CPU count is required")
            result["is_valid"] = False
            return result
        
        # Check if reserved is reasonable
        if reserved < 2:
            result["warnings"].append(
                "Reserved CPU count is very low. Minimum 2 CPUs recommended for system workloads."
            )
            result["recommendations"].append("Consider reserving at least 2-4 CPUs for system processes")
        
        # Calculate isolated if not provided
        if isolated is None:
            isolated = cpus_per_node - reserved
            result["recommendations"].append(
                f"Isolated CPU count not specified. Will use {isolated} CPUs (total - reserved)"
            )
        
        # Check if total fits
        total_required = isolated + reserved
        if total_required > cpus_per_node:
            result["is_valid"] = False
            result["errors"].append(
                f"Total CPUs required ({total_required}) exceeds available CPUs per node ({cpus_per_node})"
            )
        
        # Check NUMA alignment
        if numa_nodes > 1:
            if reserved % numa_nodes != 0:
                result["warnings"].append(
                    f"Reserved CPUs ({reserved}) not evenly divisible by NUMA nodes ({numa_nodes}). "
                    "Consider using --split-reserved-cpus-across-numa flag."
                )
            result["recommendations"].append(
                f"Your system has {numa_nodes} NUMA nodes. Consider allocating CPUs aligned to NUMA boundaries "
                "for optimal performance."
            )
        
        # Check hyperthreading
        if ht_enabled:
            result["recommendations"].append(
                "Hyperthreading is enabled. For ultra-low latency workloads, "
                "consider disabling it with --disable-ht flag."
            )
        
        return result
    
    def _validate_rt_kernel(self, requirements: Dict[str, Any]) -> Dict[str, Any]:
        """Validate real-time kernel requirements."""
        result = {
            "is_valid": True,
            "warnings": [],
            "recommendations": []
        }
        
        enable_rt = requirements.get("enable_rt_kernel", False)
        architecture = self.summary.get("architectures", ["unknown"])[0]
        
        if enable_rt:
            result["recommendations"].append(
                "Real-time kernel will be enabled. This provides deterministic latency "
                "but requires node reboot during profile application."
            )
            
            # Check architecture compatibility
            if "aarch64" in architecture or "arm64" in architecture:
                result["warnings"].append(
                    "Real-time kernel on ARM architecture may have limited support. "
                    "Verify RT kernel availability for your specific hardware."
                )
            
            # Check if workload actually needs RT
            workload_type = requirements.get("workload_type", "")
            if workload_type in ["database", "ai-inference"]:
                result["warnings"].append(
                    f"RT kernel typically not required for {workload_type} workloads. "
                    "Consider disabling RT kernel to reduce complexity."
                )
        else:
            # Check if RT might be beneficial
            workload_type = requirements.get("workload_type", "")
            if workload_type in ["5g-ran", "telco-vnf"]:
                result["recommendations"].append(
                    f"RT kernel is highly recommended for {workload_type} workloads "
                    "to achieve deterministic low latency."
                )
        
        return result
    
    def _validate_hugepages(self, requirements: Dict[str, Any]) -> Dict[str, Any]:
        """Validate hugepages requirements."""
        result = {
            "is_valid": True,
            "errors": [],
            "recommendations": []
        }
        
        hugepage_size = requirements.get("hugepages_size")
        hugepage_count = requirements.get("hugepages_count")
        architecture = self.summary.get("architectures", ["unknown"])[0]
        
        if hugepage_size:
            # Validate hugepage size for architecture
            valid_sizes_x86 = ["2M", "1G"]
            valid_sizes_arm = ["64K", "2M", "32M", "512M", "1G"]
            
            if "x86" in architecture or "amd64" in architecture:
                if hugepage_size not in valid_sizes_x86:
                    result["errors"].append(
                        f"Invalid hugepage size '{hugepage_size}' for x86_64. "
                        f"Valid sizes: {', '.join(valid_sizes_x86)}"
                    )
                    result["is_valid"] = False
            elif "aarch64" in architecture or "arm64" in architecture:
                if hugepage_size not in valid_sizes_arm:
                    result["errors"].append(
                        f"Invalid hugepage size '{hugepage_size}' for aarch64. "
                        f"Valid sizes: {', '.join(valid_sizes_arm)}"
                    )
                    result["is_valid"] = False
            
            # Check if count is specified
            if hugepage_count:
                # Estimate memory usage
                size_gb = self._hugepage_to_gb(hugepage_size)
                total_gb = size_gb * hugepage_count
                
                result["recommendations"].append(
                    f"Hugepages will reserve {total_gb:.2f} GB of memory. "
                    "Ensure sufficient memory is available."
                )
            else:
                result["recommendations"].append(
                    "Hugepage count not specified. Define the number of hugepages needed for your workload."
                )
        
        # Workload-specific recommendations
        workload_type = requirements.get("workload_type", "")
        enable_dpdk = requirements.get("enable_dpdk", False)
        
        if enable_dpdk and not hugepage_size:
            result["recommendations"].append(
                "DPDK workloads typically require hugepages (1G recommended). "
                "Consider configuring hugepages for optimal performance."
            )
        
        return result
    
    def _validate_power_mode(self, requirements: Dict[str, Any]) -> Dict[str, Any]:
        """Validate power mode settings."""
        result = {
            "warnings": [],
            "recommendations": []
        }
        
        power_mode = requirements.get("power_mode", "default")
        per_pod_pm = requirements.get("per_pod_power_management", False)
        
        if power_mode in ["low-latency", "ultra-low-latency"]:
            result["warnings"].append(
                f"Power mode '{power_mode}' will increase power consumption significantly. "
                "Ensure adequate cooling and power capacity."
            )
        
        if per_pod_pm and power_mode != "default":
            result["warnings"].append(
                "Per-pod power management and high power consumption mode are mutually exclusive. "
                "Per-pod power management will be disabled."
            )
        
        return result
    
    def _hugepage_to_gb(self, size: str) -> float:
        """Convert hugepage size string to GB."""
        size_upper = size.upper()
        if size_upper.endswith("G"):
            return float(size_upper[:-1])
        elif size_upper.endswith("M"):
            return float(size_upper[:-1]) / 1024
        elif size_upper.endswith("K"):
            return float(size_upper[:-1]) / (1024 * 1024)
        return 0


