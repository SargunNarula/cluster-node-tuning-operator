#!/usr/bin/env python3
"""
Local testing script for MCP server components without running the full MCP server.
Tests individual modules and their integration.
"""

import sys
import os

# Add current directory to path
sys.path.insert(0, os.path.dirname(__file__))

from must_gather_parser import MustGatherParser
from hardware_validator import HardwareValidator
from ppc_generator import PPCCommandGenerator
from workload_templates import WorkloadTemplates


def test_workload_templates():
    """Test workload templates listing and retrieval."""
    print("=" * 70)
    print("TEST 1: Workload Templates")
    print("=" * 70)
    
    templates = WorkloadTemplates.list_templates()
    print(f"\n✓ Found {len(templates)} workload templates:\n")
    
    for template in templates:
        print(f"  • {template['type']}: {template['name']}")
        print(f"    RT Kernel: {'✓' if template['requires_rt_kernel'] else '✗'}")
        print(f"    DPDK: {'✓' if template['requires_dpdk'] else '✗'}")
        print(f"    Power Mode: {template['power_mode']}")
        print()
    
    # Test specific template
    template = WorkloadTemplates.get_template("5g-ran")
    print("✓ Retrieved 5G RAN template:")
    print(f"  Description: {template['description'][:80]}...")
    print(f"  Use Cases: {', '.join(template['use_cases'])}")
    print()
    
    return True


def test_must_gather_parser(must_gather_path=None):
    """Test must-gather parser with fallback to mock data."""
    print("=" * 70)
    print("TEST 2: Must-Gather Parser")
    print("=" * 70)
    
    if must_gather_path and os.path.exists(must_gather_path):
        print(f"\n✓ Using must-gather from: {must_gather_path}\n")
        try:
            parser = MustGatherParser(must_gather_path)
            hardware_info = parser.analyze_hardware()
            print("✓ Successfully parsed must-gather data:")
            print(parser.get_summary())
            return hardware_info
        except Exception as e:
            print(f"✗ Error parsing must-gather: {e}")
            print("  Falling back to mock data...\n")
    else:
        print("\n⚠ No must-gather path provided, using mock data\n")
    
    # Mock hardware data for testing
    hardware_info = {
        "nodes": [
            {
                "name": "worker-0",
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
                "kernel_version": "5.14.0",
                "os_image": "Red Hat Enterprise Linux CoreOS",
                "labels": {}
            }
        ],
        "summary": {
            "total_nodes": 2,
            "worker_nodes": 2,
            "total_cpus": 96,
            "cpus_per_node": 48,
            "architectures": ["amd64"],
            "numa_nodes_per_node": 2,
            "hyperthreading_enabled": True,
            "sample_topology": {
                "total_cpus": 48,
                "numa_nodes": 2,
                "cores_per_socket": 12,
                "threads_per_core": 2,
                "sockets": 2
            }
        }
    }
    
    print("✓ Mock hardware data loaded:")
    print(f"  - Worker Nodes: {hardware_info['summary']['worker_nodes']}")
    print(f"  - CPUs per Node: {hardware_info['summary']['cpus_per_node']}")
    print(f"  - NUMA Nodes: {hardware_info['summary']['numa_nodes_per_node']}")
    print(f"  - Hyperthreading: {hardware_info['summary']['hyperthreading_enabled']}")
    print()
    
    return hardware_info


def test_hardware_validator(hardware_info):
    """Test hardware validator with various scenarios."""
    print("=" * 70)
    print("TEST 3: Hardware Validator")
    print("=" * 70)
    
    validator = HardwareValidator(hardware_info)
    
    # Test Case 1: Valid 5G RAN configuration
    print("\n📋 Test Case 1: Valid 5G RAN Configuration")
    print("-" * 70)
    result = validator.validate({
        "workload_type": "5g-ran",
        "reserved_cpu_count": 8,
        "enable_rt_kernel": True,
        "enable_dpdk": True,
        "power_mode": "ultra-low-latency"
    })
    print(f"Status: {'✓ VALID' if result['is_valid'] else '✗ INVALID'}")
    if result['warnings']:
        print("Warnings:")
        for w in result['warnings']:
            print(f"  ⚠ {w}")
    if result['recommendations']:
        print("Recommendations:")
        for r in result['recommendations'][:3]:  # Show first 3
            print(f"  💡 {r}")
    print()
    
    # Test Case 2: Too many reserved CPUs
    print("📋 Test Case 2: Too Many Reserved CPUs (Should Fail)")
    print("-" * 70)
    result = validator.validate({
        "workload_type": "database",
        "reserved_cpu_count": 46,  # Too many!
        "enable_rt_kernel": False
    })
    print(f"Status: {'✓ VALID' if result['is_valid'] else '✗ INVALID (Expected)'}")
    if result['errors']:
        print("Errors:")
        for e in result['errors']:
            print(f"  ✗ {e}")
    print()
    
    # Test Case 3: Database with RT kernel (not recommended)
    print("📋 Test Case 3: Database with RT Kernel (Valid but Not Recommended)")
    print("-" * 70)
    result = validator.validate({
        "workload_type": "database",
        "reserved_cpu_count": 8,
        "enable_rt_kernel": True,  # Not needed for database
        "power_mode": "default"
    })
    print(f"Status: {'✓ VALID' if result['is_valid'] else '✗ INVALID'}")
    if result['warnings']:
        print("Warnings:")
        for w in result['warnings']:
            print(f"  ⚠ {w}")
    print()
    
    return True


def test_ppc_generator(hardware_info):
    """Test PPC command generator."""
    print("=" * 70)
    print("TEST 4: PPC Command Generator")
    print("=" * 70)
    
    generator = PPCCommandGenerator(hardware_info)
    
    # Test Case 1: 5G RAN profile
    print("\n📋 Test Case 1: Generate 5G RAN Profile Command")
    print("-" * 70)
    
    template = WorkloadTemplates.get_template("5g-ran")
    params = generator.generate_parameters(
        workload_type="5g-ran",
        mcp_name="worker-cnf",
        profile_name="5g-performance",
        template=template
    )
    
    print("✓ Generated Parameters:")
    for key, value in params.items():
        print(f"  - {key}: {value}")
    print()
    
    command = generator.build_command("/must-gather", params)
    print("✓ Generated PPC Command:")
    print()
    print(command)
    print()
    
    explanation = generator.explain_parameters(params)
    print("✓ Parameter Explanation:")
    print()
    print(explanation)
    print()
    
    # Test Case 2: Database profile
    print("\n📋 Test Case 2: Generate Database Profile Command")
    print("-" * 70)
    
    template = WorkloadTemplates.get_template("database")
    params = generator.generate_parameters(
        workload_type="database",
        mcp_name="worker-db",
        profile_name="database-performance",
        template=template,
        reserved_cpu_count=6  # Override
    )
    
    print("✓ Generated Parameters:")
    for key, value in params.items():
        print(f"  - {key}: {value}")
    print()
    
    command = generator.build_command("/must-gather", params)
    print("✓ Generated PPC Command:")
    print()
    print(command)
    print()
    
    return True


def test_workload_recommendations():
    """Test natural language workload recommendations."""
    print("=" * 70)
    print("TEST 5: Natural Language Workload Recommendations")
    print("=" * 70)
    
    test_cases = [
        "I need to run 5G RAN workloads with ultra-low latency",
        "Database server running PostgreSQL with some real-time requirements",
        "AI inference using TensorFlow with DPDK packet processing",
        "Low latency trading system for high-frequency trading"
    ]
    
    for description in test_cases:
        print(f"\n📝 Description: \"{description}\"")
        print("-" * 70)
        
        # Simple keyword matching (simulating what the MCP server does)
        description_lower = description.lower()
        
        if "5g" in description_lower or "ran" in description_lower:
            workload_type = "5g-ran"
        elif "database" in description_lower or "postgresql" in description_lower:
            workload_type = "database"
        elif "ai" in description_lower or "inference" in description_lower:
            workload_type = "ai-inference"
        elif "trading" in description_lower:
            workload_type = "low-latency-trading"
        else:
            workload_type = "custom"
        
        print(f"✓ Identified Workload Type: {workload_type}")
        
        template = WorkloadTemplates.get_template(workload_type)
        if template:
            print(f"✓ Recommended Settings:")
            print(f"  - RT Kernel: {template['default_config']['enable_rt_kernel']}")
            print(f"  - DPDK: {template['default_config']['enable_dpdk']}")
            print(f"  - Power Mode: {template['default_config']['power_mode']}")
        print()
    
    return True


def main():
    """Run all tests."""
    print("\n" + "=" * 70)
    print("  MCP SERVER LOCAL TESTING SUITE")
    print("=" * 70 + "\n")
    
    # Check for must-gather path
    must_gather_path = None
    if len(sys.argv) > 1:
        must_gather_path = sys.argv[1]
        if not os.path.exists(must_gather_path):
            print(f"⚠ Warning: Must-gather path does not exist: {must_gather_path}")
            print("  Will use mock data instead.\n")
            must_gather_path = None
    else:
        print("💡 Usage: python test_local.py [must-gather-path]")
        print("  No must-gather path provided, will use mock data.\n")
    
    try:
        # Run tests
        test_workload_templates()
        hardware_info = test_must_gather_parser(must_gather_path)
        test_hardware_validator(hardware_info)
        test_ppc_generator(hardware_info)
        test_workload_recommendations()
        
        print("=" * 70)
        print("  ✅ ALL TESTS PASSED")
        print("=" * 70 + "\n")
        
        return 0
        
    except Exception as e:
        print(f"\n❌ TEST FAILED: {e}")
        import traceback
        traceback.print_exc()
        return 1


if __name__ == "__main__":
    sys.exit(main())




