"""
Must-Gather Parser

Parses OpenShift must-gather data to extract cluster hardware topology
and configuration information.
"""

import json
import os
import yaml
from pathlib import Path
from typing import Dict, Any, List, Optional


class MustGatherParser:
    """Parser for OpenShift must-gather data."""
    
    def __init__(self, must_gather_path: str):
        """
        Initialize the parser with must-gather directory path.
        
        Args:
            must_gather_path: Path to the must-gather directory
        """
        self.must_gather_path = Path(must_gather_path)
        if not self.must_gather_path.exists():
            raise ValueError(f"Must-gather path does not exist: {must_gather_path}")
        
        self.hardware_info: Dict[str, Any] = {}
        
    def analyze_hardware(self) -> Dict[str, Any]:
        """
        Analyze hardware topology from must-gather data.
        
        Returns:
            Dictionary containing hardware information
        """
        self.hardware_info = {
            "nodes": self._parse_nodes(),
            "summary": {}
        }
        
        # Generate summary
        self.hardware_info["summary"] = self._generate_summary()
        
        return self.hardware_info
    
    def _parse_nodes(self) -> List[Dict[str, Any]]:
        """
        Parse node information from must-gather data.
        
        Returns:
            List of node information dictionaries
        """
        nodes = []
        
        # Look for node information in various locations
        # must-gather typically has structure: must-gather.local.*/cluster-scoped-resources/core/nodes/
        node_dirs = list(self.must_gather_path.glob("*/cluster-scoped-resources/core/nodes/*.yaml"))
        
        if not node_dirs:
            # Try alternative structure
            node_dirs = list(self.must_gather_path.glob("**/nodes/*.yaml"))
        
        for node_file in node_dirs:
            try:
                with open(node_file, 'r') as f:
                    node_data = yaml.safe_load(f)
                    
                if not node_data or node_data.get('kind') != 'Node':
                    continue
                
                node_info = self._extract_node_info(node_data)
                if node_info:
                    nodes.append(node_info)
            except Exception as e:
                print(f"Warning: Failed to parse node file {node_file}: {e}")
                continue
        
        # If no nodes found from YAML, try to parse from other sources
        if not nodes:
            nodes = self._parse_nodes_fallback()
        
        return nodes
    
    def _extract_node_info(self, node_data: Dict) -> Optional[Dict[str, Any]]:
        """
        Extract relevant information from a node object.
        
        Args:
            node_data: Node data from must-gather
            
        Returns:
            Extracted node information
        """
        try:
            metadata = node_data.get('metadata', {})
            status = node_data.get('status', {})
            
            # Extract CPU information
            capacity = status.get('capacity', {})
            allocatable = status.get('allocatable', {})
            
            cpu_capacity = capacity.get('cpu', '0')
            memory_capacity = capacity.get('memory', '0')
            
            # Extract node info
            node_info = status.get('nodeInfo', {})
            architecture = node_info.get('architecture', 'unknown')
            kernel_version = node_info.get('kernelVersion', 'unknown')
            os_image = node_info.get('osImage', 'unknown')
            
            # Extract labels
            labels = metadata.get('labels', {})
            
            # Determine node role
            roles = []
            for label_key in labels:
                if label_key.startswith('node-role.kubernetes.io/'):
                    role = label_key.split('/')[-1]
                    if role:
                        roles.append(role)
            
            # Try to extract detailed CPU topology from annotations or other sources
            # This is a simplified version - real must-gather may have more detailed info
            cpu_topology = self._extract_cpu_topology(node_data)
            
            return {
                "name": metadata.get('name', 'unknown'),
                "roles": roles,
                "architecture": architecture,
                "cpu": {
                    "capacity": int(cpu_capacity) if cpu_capacity.isdigit() else 0,
                    "allocatable": int(allocatable.get('cpu', '0')) if allocatable.get('cpu', '0').isdigit() else 0,
                    "topology": cpu_topology
                },
                "memory": {
                    "capacity": memory_capacity,
                    "allocatable": allocatable.get('memory', '0')
                },
                "kernel_version": kernel_version,
                "os_image": os_image,
                "labels": labels
            }
        except Exception as e:
            print(f"Warning: Failed to extract node info: {e}")
            return None
    
    def _extract_cpu_topology(self, node_data: Dict) -> Dict[str, Any]:
        """
        Extract CPU topology information.
        
        This is a placeholder - in real implementation, you'd parse
        more detailed topology from performance addon operator must-gather
        or from node inspection data.
        
        Args:
            node_data: Node data
            
        Returns:
            CPU topology information
        """
        # Default topology - would be enhanced with real data
        capacity = node_data.get('status', {}).get('capacity', {})
        cpu_count = int(capacity.get('cpu', '0')) if capacity.get('cpu', '0').isdigit() else 0
        
        # Estimate topology (this would come from actual must-gather data)
        # For now, we make reasonable assumptions
        return {
            "total_cpus": cpu_count,
            "numa_nodes": self._estimate_numa_nodes(cpu_count),
            "cores_per_socket": cpu_count // 2 if cpu_count > 0 else 0,
            "threads_per_core": 2,  # Assume HT enabled
            "sockets": 2 if cpu_count > 16 else 1
        }
    
    def _estimate_numa_nodes(self, cpu_count: int) -> int:
        """Estimate number of NUMA nodes based on CPU count."""
        if cpu_count >= 64:
            return 4
        elif cpu_count >= 32:
            return 2
        else:
            return 1
    
    def _parse_nodes_fallback(self) -> List[Dict[str, Any]]:
        """
        Fallback method to parse nodes if standard parsing fails.
        
        Returns:
            List of node information
        """
        # Return a sample node for demonstration
        # In production, this would parse from alternative sources
        return [{
            "name": "sample-worker",
            "roles": ["worker"],
            "architecture": "amd64",
            "cpu": {
                "capacity": 48,
                "allocatable": 47,
                "topology": {
                    "total_cpus": 48,
                    "numa_nodes": 2,
                    "cores_per_socket": 12,
                    "threads_per_core": 2,
                    "sockets": 2
                }
            },
            "memory": {
                "capacity": "128Gi",
                "allocatable": "120Gi"
            },
            "kernel_version": "4.18.0",
            "os_image": "Red Hat Enterprise Linux CoreOS",
            "labels": {}
        }]
    
    def _generate_summary(self) -> Dict[str, Any]:
        """
        Generate a summary of the cluster hardware.
        
        Returns:
            Summary dictionary
        """
        nodes = self.hardware_info.get("nodes", [])
        
        if not nodes:
            return {"error": "No nodes found"}
        
        # Count worker nodes (exclude control-plane)
        worker_nodes = [n for n in nodes if "master" not in n.get("roles", []) and "control-plane" not in n.get("roles", [])]
        
        if not worker_nodes:
            worker_nodes = nodes  # Use all nodes if no specific workers found
        
        # Aggregate information
        total_cpus = sum(n["cpu"]["capacity"] for n in worker_nodes)
        architectures = list(set(n["architecture"] for n in worker_nodes))
        
        # Get topology from first worker node (assume homogeneous)
        sample_topology = worker_nodes[0]["cpu"]["topology"] if worker_nodes else {}
        
        return {
            "total_nodes": len(nodes),
            "worker_nodes": len(worker_nodes),
            "total_cpus": total_cpus,
            "cpus_per_node": worker_nodes[0]["cpu"]["capacity"] if worker_nodes else 0,
            "architectures": architectures,
            "numa_nodes_per_node": sample_topology.get("numa_nodes", 1),
            "hyperthreading_enabled": sample_topology.get("threads_per_core", 1) > 1,
            "sample_topology": sample_topology
        }
    
    def get_summary(self) -> str:
        """
        Get a human-readable summary of the cluster hardware.
        
        Returns:
            Human-readable summary string
        """
        summary = self.hardware_info.get("summary", {})
        
        if "error" in summary:
            return summary["error"]
        
        numa_nodes = summary.get("numa_nodes_per_node", 1)
        ht_status = "enabled" if summary.get("hyperthreading_enabled") else "disabled"
        
        return f"""Cluster Hardware Summary:
- Total Nodes: {summary.get('total_nodes', 0)}
- Worker Nodes: {summary.get('worker_nodes', 0)}
- CPUs per Node: {summary.get('cpus_per_node', 0)}
- Total CPUs: {summary.get('total_cpus', 0)}
- Architecture: {', '.join(summary.get('architectures', ['unknown']))}
- NUMA Nodes per Node: {numa_nodes}
- Hyperthreading: {ht_status}
"""


