"""
Workload Templates

Predefined templates for common workload types with recommended configurations.
"""

from typing import Dict, Any, List, Optional


class WorkloadTemplates:
    """Provides templates for common workload types."""
    
    TEMPLATES = {
        "5g-ran": {
            "name": "5G RAN (Radio Access Network)",
            "description": (
                "Optimized for 5G Radio Access Network workloads requiring "
                "ultra-low latency and deterministic performance. "
                "Uses real-time kernel, disabled hyperthreading, and aggressive power settings."
            ),
            "default_config": {
                "enable_rt_kernel": True,
                "disable_ht": True,
                "enable_dpdk": True,
                "power_mode": "ultra-low-latency",
                "topology_policy": "single-numa-node",
                "split_reserved_across_numa": True,
                "per_pod_power_management": False
            },
            "recommended_hugepages": {
                "size": "1G",
                "note": "1G hugepages recommended for DPDK packet processing"
            },
            "use_cases": [
                "5G base station processing",
                "Real-time radio signal processing",
                "Ultra-low latency packet processing"
            ]
        },
        
        "telco-vnf": {
            "name": "Telco VNF (Virtual Network Function)",
            "description": (
                "Optimized for Telco VNF workloads including packet processing, "
                "session border controllers, and network functions. "
                "Enables RT kernel and DPDK for high-performance networking."
            ),
            "default_config": {
                "enable_rt_kernel": True,
                "disable_ht": False,
                "enable_dpdk": True,
                "power_mode": "low-latency",
                "topology_policy": "single-numa-node",
                "split_reserved_across_numa": True,
                "per_pod_power_management": False
            },
            "recommended_hugepages": {
                "size": "1G",
                "note": "1G hugepages recommended for DPDK"
            },
            "use_cases": [
                "Virtual routers",
                "Session border controllers",
                "Packet gateways",
                "DPI (Deep Packet Inspection)"
            ]
        },
        
        "database": {
            "name": "Database Server",
            "description": (
                "Optimized for database workloads requiring memory optimization "
                "and consistent performance. Uses hugepages for memory management "
                "but does not require RT kernel."
            ),
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
                "note": "2M hugepages for database shared memory buffers"
            },
            "use_cases": [
                "PostgreSQL",
                "MySQL/MariaDB",
                "MongoDB",
                "Oracle Database"
            ]
        },
        
        "ai-inference": {
            "name": "AI/ML Inference",
            "description": (
                "Optimized for AI/ML inference workloads requiring CPU isolation "
                "and consistent performance. Does not use RT kernel but isolates "
                "CPUs for predictable inference latency."
            ),
            "default_config": {
                "enable_rt_kernel": False,
                "disable_ht": False,
                "enable_dpdk": False,
                "power_mode": "low-latency",
                "topology_policy": "single-numa-node",
                "split_reserved_across_numa": True,
                "per_pod_power_management": False
            },
            "recommended_hugepages": {
                "size": "2M",
                "note": "2M hugepages for model data structures"
            },
            "use_cases": [
                "TensorFlow Serving",
                "PyTorch inference",
                "ONNX Runtime",
                "Real-time AI inference"
            ]
        },
        
        "hpc": {
            "name": "High Performance Computing",
            "description": (
                "Optimized for HPC workloads requiring maximum compute throughput "
                "and NUMA awareness. Focuses on CPU isolation and memory locality."
            ),
            "default_config": {
                "enable_rt_kernel": False,
                "disable_ht": False,
                "enable_dpdk": False,
                "power_mode": "low-latency",
                "topology_policy": "single-numa-node",
                "split_reserved_across_numa": True,
                "per_pod_power_management": False
            },
            "recommended_hugepages": {
                "size": "1G",
                "note": "1G hugepages for large memory allocations"
            },
            "use_cases": [
                "Scientific computing",
                "Computational fluid dynamics",
                "Molecular dynamics",
                "Weather simulation"
            ]
        },
        
        "low-latency-trading": {
            "name": "Low-Latency Trading",
            "description": (
                "Optimized for financial trading systems requiring microsecond latency. "
                "Uses RT kernel, disabled HT, and ultra-low-latency power mode."
            ),
            "default_config": {
                "enable_rt_kernel": True,
                "disable_ht": True,
                "enable_dpdk": True,
                "power_mode": "ultra-low-latency",
                "topology_policy": "single-numa-node",
                "split_reserved_across_numa": True,
                "per_pod_power_management": False
            },
            "recommended_hugepages": {
                "size": "1G",
                "note": "1G hugepages for lock-free data structures"
            },
            "use_cases": [
                "High-frequency trading",
                "Market data processing",
                "Order matching engines",
                "Risk calculation engines"
            ]
        },
        
        "media-processing": {
            "name": "Media Processing",
            "description": (
                "Optimized for real-time media encoding/transcoding workloads. "
                "Balances between throughput and latency."
            ),
            "default_config": {
                "enable_rt_kernel": False,
                "disable_ht": False,
                "enable_dpdk": False,
                "power_mode": "default",
                "topology_policy": "restricted",
                "split_reserved_across_numa": True,
                "per_pod_power_management": True
            },
            "recommended_hugepages": {
                "size": "2M",
                "note": "2M hugepages for frame buffers"
            },
            "use_cases": [
                "Video transcoding",
                "Live streaming",
                "Real-time video processing",
                "Audio processing"
            ]
        }
    }
    
    @classmethod
    def get_template(cls, workload_type: str) -> Optional[Dict[str, Any]]:
        """
        Get template for a specific workload type.
        
        Args:
            workload_type: Type of workload
            
        Returns:
            Template dictionary or None if not found
        """
        return cls.TEMPLATES.get(workload_type)
    
    @classmethod
    def list_templates(cls) -> List[Dict[str, Any]]:
        """
        List all available templates.
        
        Returns:
            List of template summaries
        """
        templates = []
        for key, template in cls.TEMPLATES.items():
            templates.append({
                "type": key,
                "name": template["name"],
                "description": template["description"],
                "use_cases": template.get("use_cases", []),
                "requires_rt_kernel": template["default_config"]["enable_rt_kernel"],
                "requires_dpdk": template["default_config"]["enable_dpdk"],
                "power_mode": template["default_config"]["power_mode"]
            })
        return templates
    
    @classmethod
    def get_template_names(cls) -> List[str]:
        """
        Get list of all template names.
        
        Returns:
            List of template type identifiers
        """
        return list(cls.TEMPLATES.keys())
    
    @classmethod
    def find_template_by_keywords(cls, keywords: List[str]) -> List[str]:
        """
        Find templates matching keywords.
        
        Args:
            keywords: List of keywords to search for
            
        Returns:
            List of matching template types
        """
        matches = []
        keywords_lower = [k.lower() for k in keywords]
        
        for key, template in cls.TEMPLATES.items():
            # Search in description and use cases
            search_text = (
                template["description"].lower() + " " +
                " ".join(template.get("use_cases", [])).lower() +
                " " + template["name"].lower()
            )
            
            if any(keyword in search_text for keyword in keywords_lower):
                matches.append(key)
        
        return matches


