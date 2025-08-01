/*
 * Licensed under the Apache License, Version 2.0 (the "License");
 * you may not use this file except in compliance with the License.
 * You may obtain a copy of the License at
 *
 *     http://www.apache.org/licenses/LICENSE-2.0
 *
 * Unless required by applicable law or agreed to in writing, software
 * distributed under the License is distributed on an "AS IS" BASIS,
 * WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
 * See the License for the specific language governing permissions and
 * limitations under the License.
 *
 * Copyright 2021 Red Hat, Inc.
 */

package profilecreator

import (
	"fmt"
	"os"
	"path"
	"reflect"
	"sort"

	"github.com/jaypipes/ghw/pkg/cpu"
	"github.com/jaypipes/ghw/pkg/topology"

	configv1 "github.com/openshift/api/config/v1"
	k8syaml "k8s.io/apimachinery/pkg/util/yaml"
	"k8s.io/utils/cpuset"

	"github.com/openshift/cluster-node-tuning-operator/pkg/performanceprofile/profilecreator/toleration"
)

const (
	// noSMTKernelArg is the kernel arg value to disable SMT in a system
	noSMTKernelArg = "nosmt"
	// allCores correspond to the value when all the processorCores need to be added to the generated CPUset
	allCores = -1
)

var (
	// This filter is used to avoid offlining the first logical processor of each core.
	// LogicalProcessors is a slice of integers representing the logical processor IDs assigned to
	// a processing unit for a core. GHW API guarantees that the logicalProcessors correspond
	// to hyperthread pairs and in the code below we select only the first hyperthread (id=0)
	// of the available logical processors.
	// Please refer to https://www.kernel.org/doc/Documentation/x86/topology.txt for more information on
	// x86 hardware topology. This document clarifies the main aspects of x86 topology modeling and
	// representation in the linux kernel and explains why we select id=0 for obtaining the first
	// hyperthread (logical core).
	filterFirstLogicalProcessorInCore = func(index, lpID int) bool { return index != 0 }
)

// topologyHTDisabled returns topologyinfo in case Hyperthreading needs to be disabled.
// It receives a pointer to Topology.Info and deletes logicalprocessors from individual cores.
// The behavior of this function depends on ghw data representation.
func topologyHTDisabled(info *topology.Info) *topology.Info {
	disabledHTTopology := &topology.Info{
		Architecture: info.Architecture,
	}
	newNodes := []*topology.Node{}
	for _, node := range info.Nodes {
		var newNode *topology.Node
		cores := []*cpu.ProcessorCore{}
		for _, processorCore := range node.Cores {
			newCore := cpu.ProcessorCore{ID: processorCore.ID,
				NumThreads: 1,
			}
			// LogicalProcessors is a slice of ints representing the logical processor IDs assigned to
			// a processing unit for a core. GHW API guarantees that the logicalProcessors correspond
			// to hyperthread pairs and in the code below we select only the first hyperthread (id=0)
			// of the available logical processors.
			for id, logicalProcessor := range processorCore.LogicalProcessors {
				// Please refer to https://www.kernel.org/doc/Documentation/x86/topology.txt for more information on
				// x86 hardware topology. This document clarifies the main aspects of x86 topology modeling and
				// representation in the linux kernel and explains why we select id=0 for obtaining the first
				// hyperthread (logical core).
				if id == 0 {
					newCore.LogicalProcessors = []int{logicalProcessor}
					cores = append(cores, &newCore)
				}
			}
			newNode = &topology.Node{Cores: cores,
				ID: node.ID,
			}
		}
		newNodes = append(newNodes, newNode)
		disabledHTTopology.Nodes = newNodes
	}
	return disabledHTTopology
}

type extendedCPUInfo struct {
	CpuInfo *cpu.Info
	// Number of logicalprocessors already reserved for each Processor (aka Socket)
	NumLogicalProcessorsUsed map[int]int
	LogicalProcessorsUsed    map[int]struct{}
}

type systemInfo struct {
	CpuInfo      *extendedCPUInfo
	TopologyInfo *topology.Info
	HtEnabled    bool
}

// Calculates the resevered, isolated and offlined cpuSets.
func CalculateCPUSets(systemInfo *systemInfo, reservedCPUCount int, offlinedCPUCount int, splitReservedCPUsAcrossNUMA bool, disableHTFlag bool, highPowerConsumptionMode bool) (cpuset.CPUSet, cpuset.CPUSet, cpuset.CPUSet, error) {
	topologyInfo := systemInfo.TopologyInfo
	htEnabled := systemInfo.HtEnabled

	// Need to update Topology info to avoid using sibling Logical processors
	// if user want to "disable" them in the kernel
	updatedTopologyInfo, err := updateTopologyInfo(topologyInfo, disableHTFlag, systemInfo.HtEnabled)
	if err != nil {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, err
	}

	updatedExtCPUInfo, err := updateExtendedCPUInfo(systemInfo.CpuInfo, cpuset.CPUSet{}, disableHTFlag, htEnabled)
	if err != nil {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, err
	}

	cpuInfo := updatedExtCPUInfo.CpuInfo
	// Check limits are in range
	if reservedCPUCount <= 0 || reservedCPUCount >= int(cpuInfo.TotalThreads) {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, fmt.Errorf("please specify the reserved CPU count in the range [1,%d]", cpuInfo.TotalThreads-1)
	}

	if offlinedCPUCount < 0 || offlinedCPUCount >= int(cpuInfo.TotalThreads) {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, fmt.Errorf("please specify the offlined CPU count in the range [0,%d]", cpuInfo.TotalThreads-1)
	}

	if reservedCPUCount+offlinedCPUCount >= int(cpuInfo.TotalThreads) {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, fmt.Errorf("please ensure that reserved-cpu-count plus offlined-cpu-count should be in the range [0,%d]", cpuInfo.TotalThreads-1)
	}

	// Calculate reserved cpus.
	reserved, err := getReservedCPUs(updatedTopologyInfo, reservedCPUCount, splitReservedCPUsAcrossNUMA, disableHTFlag, htEnabled)
	if err != nil {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, err
	}

	updatedExtCPUInfo, err = updateExtendedCPUInfo(updatedExtCPUInfo, reserved, disableHTFlag, htEnabled)
	if err != nil {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, err
	}
	//Calculate offlined cpus
	// note this takes into account the reserved cpus from the step above
	offlined, err := getOfflinedCPUs(updatedExtCPUInfo, offlinedCPUCount, disableHTFlag, htEnabled, highPowerConsumptionMode)
	if err != nil {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, err
	}

	// Calculate isolated cpus.
	// Note that topology info could have been modified by "GetReservedCPUS" so
	// to properly calculate isolated CPUS we need to use the updated topology information.
	isolated, err := getIsolatedCPUs(updatedTopologyInfo.Nodes, reserved, offlined)
	if err != nil {
		return cpuset.CPUSet{}, cpuset.CPUSet{}, cpuset.CPUSet{}, err
	}

	return reserved, isolated, offlined, nil
}

// Calculates Isolated cpuSet as the difference between all the cpus in the topology and those already chosen as reserved or offlined.
// all cpus thar are not offlined or reserved belongs to the isolated cpuSet
func getIsolatedCPUs(topologyInfoNodes []*topology.Node, reserved, offlined cpuset.CPUSet) (cpuset.CPUSet, error) {
	total, err := totalCPUSetFromTopology(topologyInfoNodes)
	if err != nil {
		return cpuset.CPUSet{}, err
	}
	return total.Difference(reserved.Union(offlined)), nil
}

func AreAllLogicalProcessorsFromSocketUnused(extCpuInfo *extendedCPUInfo, socketId int) bool {
	if val, ok := extCpuInfo.NumLogicalProcessorsUsed[socketId]; ok {
		return val == 0
	} else {
		return true
	}
}

func getOfflinedCPUs(extCpuInfo *extendedCPUInfo, offlinedCPUCount int, disableHTFlag bool, htEnabled bool, highPowerConsumption bool) (cpuset.CPUSet, error) {
	offlined := newCPUAccumulator()
	lpOfflined := 0

	// unless we are in a high power consumption scenario
	// try to offline complete sockets first
	if !highPowerConsumption {
		cpuInfo := extCpuInfo.CpuInfo

		for _, processor := range cpuInfo.Processors {
			//can we offline a complete socket?
			if processor.NumThreads <= uint32(offlinedCPUCount-lpOfflined) && AreAllLogicalProcessorsFromSocketUnused(extCpuInfo, processor.ID) {
				acc, err := offlined.AddCores(offlinedCPUCount, processor.Cores)
				if err != nil {
					return cpuset.CPUSet{}, err
				}
				lpOfflined += acc
			}
		}
	}

	// if we still need to offline more cpus
	// try to offline sibling threads
	if lpOfflined < offlinedCPUCount {
		cpuInfo := extCpuInfo.CpuInfo

		for _, processor := range cpuInfo.Processors {
			acc, err := offlined.AddCoresWithFilter(offlinedCPUCount, processor.Cores, func(index, lpID int) bool {
				return filterFirstLogicalProcessorInCore(index, lpID) && !IsLogicalProcessorUsed(extCpuInfo, lpID)
			})
			if err != nil {
				return cpuset.CPUSet{}, err
			}

			lpOfflined += acc
		}
	}

	// if we still need to offline more cpus
	// just try to offline any cpu
	if lpOfflined < offlinedCPUCount {
		cpuInfo := extCpuInfo.CpuInfo

		for _, processor := range cpuInfo.Processors {
			acc, err := offlined.AddCoresWithFilter(offlinedCPUCount, processor.Cores, func(index, lpId int) bool {
				return !IsLogicalProcessorUsed(extCpuInfo, lpId)
			})
			if err != nil {
				return cpuset.CPUSet{}, err
			}

			lpOfflined += acc
		}
	}

	if lpOfflined < offlinedCPUCount {
		Alert("could not offline enough logical processors (required:%d, offlined:%d)", offlinedCPUCount, lpOfflined)
	}
	return offlined.Result(), nil
}

func updateTopologyInfo(topoInfo *topology.Info, disableHTFlag bool, htEnabled bool) (*topology.Info, error) {
	//currently HT is enabled on the system and the user wants to disable HT

	if htEnabled && disableHTFlag {
		Alert("Updating Topology info because currently hyperthreading is enabled and the performance profile will disable it")
		return topologyHTDisabled(topoInfo), nil
	}
	return topoInfo, nil
}

func getReservedCPUs(topologyInfo *topology.Info, reservedCPUCount int, splitReservedCPUsAcrossNUMA bool, disableHTFlag bool, htEnabled bool) (cpuset.CPUSet, error) {
	if htEnabled && disableHTFlag {
		Alert("Currently hyperthreading is enabled and the performance profile will disable it")
		htEnabled = false
	}
	Alert("NUMA cell(s): %d", len(topologyInfo.Nodes))
	totalCPUs := 0
	for id, node := range topologyInfo.Nodes {
		coreList := []int{}
		for _, core := range node.Cores {
			coreList = append(coreList, core.LogicalProcessors...)
		}
		Alert("NUMA cell %d : %v", id, coreList)
		totalCPUs += len(coreList)
	}

	Alert("CPU(s): %d", totalCPUs)

	if splitReservedCPUsAcrossNUMA {
		res, err := getCPUsSplitAcrossNUMA(reservedCPUCount, htEnabled, topologyInfo.Nodes)
		return res, err
	}
	res, err := getCPUsSequentially(reservedCPUCount, htEnabled, topologyInfo.Nodes)
	return res, err
}

type cpuAccumulator struct {
	elems map[int]struct{}
	count int
	done  bool
}

func newCPUAccumulator() *cpuAccumulator {
	return &cpuAccumulator{
		elems: map[int]struct{}{},
		count: 0,
		done:  false,
	}
}

// AddCores adds logical cores from the slice of *cpu.ProcessorCore to a CPUset till the cpuset size is equal to the max value specified
// In case the max is specified as allCores, all the cores from the slice of *cpu.ProcessorCore are added to the CPUSet
func (ca *cpuAccumulator) AddCores(max int, cores []*cpu.ProcessorCore) (int, error) {
	allLogicalProcessors := func(int, int) bool { return true }
	return ca.AddCoresWithFilter(max, cores, allLogicalProcessors)
}

func (ca *cpuAccumulator) AddCoresWithFilter(max int, cores []*cpu.ProcessorCore, filterLogicalProcessor func(int, int) bool) (int, error) {
	if ca.done {
		return -1, fmt.Errorf("CPU accumulator finalized")
	}
	initialCount := ca.count
	for _, processorCore := range cores {
		for index, logicalProcessorId := range processorCore.LogicalProcessors {
			if ca.count < max || max == allCores {
				if filterLogicalProcessor(index, logicalProcessorId) {
					_, found := ca.elems[logicalProcessorId]
					ca.elems[logicalProcessorId] = struct{}{}
					if !found {
						ca.count++
					}
				}
			}
		}
	}
	return ca.count - initialCount, nil
}

func (ca *cpuAccumulator) Result() cpuset.CPUSet {
	ca.done = true

	var keys []int
	for k := range ca.elems {
		keys = append(keys, k)
	}

	return cpuset.New(keys...)
}

// getCPUsSplitAcrossNUMA returns Reserved and Isolated CPUs split across NUMA nodes
// We identify the right number of CPUs that need to be allocated per NUMA node, meaning reservedPerNuma + (the additional number based on the remainder and the NUMA node)
// E.g. If the user requests 15 reserved cpus and we have 4 numa nodes, we find reservedPerNuma in this case is 3 and remainder = 3.
// For each numa node we find a max which keeps track of the cumulative resources that should be allocated for each NUMA node:
// max = (numaID+1)*reservedPerNuma + (numaNodeNum - remainder)
// For NUMA node 0 max = (0+1)*3 + 4-3 = 4 remainder is decremented => remainder is 2
// For NUMA node 1 max = (1+1)*3 + 4-2 = 8 remainder is decremented => remainder is 1
// For NUMA node 2 max = (2+1)*3 + 4-2 = 12 remainder is decremented => remainder is 0
// For NUMA Node 3 remainder = 0 so max = 12 + 3 = 15.
func getCPUsSplitAcrossNUMA(reservedCPUCount int, htEnabled bool, topologyInfoNodes []*topology.Node) (cpuset.CPUSet, error) {
	reservedCPUs := newCPUAccumulator()

	numaNodeNum := len(topologyInfoNodes)

	max := 0
	reservedPerNuma := reservedCPUCount / numaNodeNum
	remainder := reservedCPUCount % numaNodeNum
	if remainder != 0 {
		Alert("The reserved CPUs cannot be split equally across NUMA Nodes")
	}
	for numaID, node := range topologyInfoNodes {
		if remainder != 0 {
			max = (numaID+1)*reservedPerNuma + (numaNodeNum - remainder)
			remainder--
		} else {
			max = max + reservedPerNuma
		}
		if max%2 != 0 && htEnabled {
			return reservedCPUs.Result(), fmt.Errorf("can't allocate odd number of CPUs from a NUMA Node")
		}
		if _, err := reservedCPUs.AddCores(max, node.Cores); err != nil {
			return cpuset.CPUSet{}, err
		}
	}

	return reservedCPUs.Result(), nil
}

func getCPUsSequentially(reservedCPUCount int, htEnabled bool, topologyInfoNodes []*topology.Node) (cpuset.CPUSet, error) {
	reservedCPUs := newCPUAccumulator()

	if reservedCPUCount%2 != 0 && htEnabled {
		return reservedCPUs.Result(), fmt.Errorf("can't allocate odd number of CPUs from a NUMA Node")
	}
	for _, node := range topologyInfoNodes {
		if _, err := reservedCPUs.AddCores(reservedCPUCount, node.Cores); err != nil {
			return cpuset.CPUSet{}, err
		}
	}
	return reservedCPUs.Result(), nil
}

func totalCPUSetFromTopology(topologyInfoNodes []*topology.Node) (cpuset.CPUSet, error) {
	totalCPUs := newCPUAccumulator()
	for _, node := range topologyInfoNodes {
		//all the cores from node.Cores need to be added, hence allCores is specified as the max value
		if _, err := totalCPUs.AddCores(allCores, node.Cores); err != nil {
			return cpuset.CPUSet{}, err
		}
	}
	return totalCPUs.Result(), nil
}

// IsHyperthreadingEnabled checks if hyperthreading is enabled on the system or not
func (ghwHandler GHWHandler) IsHyperthreadingEnabled() (bool, error) {
	cpuInfo, err := ghwHandler.CPU()
	if err != nil {
		return false, fmt.Errorf("can't obtain CPU Info from GHW snapshot: %v", err)
	}
	// Since there is no way to disable flags per-processor (not system wide) we check the flags of the first available processor.
	// A following implementation will leverage the /sys/devices/system/cpu/smt/active file which is the "standard" way to query HT.
	return cpuInfo.TotalCores != cpuInfo.TotalHardwareThreads, nil
}

// EnsureNodesHaveTheSameHardware returns an error if all the input nodes do not have the same hardware configuration and
// updates the toleration set to consider as warnings/comments when publishing the generated profile
func EnsureNodesHaveTheSameHardware(nodeHandlers []*GHWHandler, tols toleration.Set) error {
	if len(nodeHandlers) < 1 {
		return fmt.Errorf("no suitable nodes to compare")
	}

	firstHandle := nodeHandlers[0]
	firstTopology, err := firstHandle.SortedTopology()
	if err != nil {
		return fmt.Errorf("can't obtain Topology info from GHW snapshot for %s: %v", firstHandle.Node.GetName(), err)
	}
	for _, handle := range nodeHandlers[1:] {
		topology, err := handle.SortedTopology()
		if err != nil {
			return fmt.Errorf("can't obtain Topology info from GHW snapshot for %s: %v", handle.Node.GetName(), err)
		}
		if err := ensureSameTopology(firstTopology, topology, tols); err != nil {
			return fmt.Errorf("nodes %s and %s have different topology: %v", firstHandle.Node.GetName(), handle.Node.GetName(), err)
		}
	}

	return nil
}

func ensureSameTopology(topology1, topology2 *topology.Info, tols toleration.Set) error {
	// the assumption here is that both topologies are deep sorted (e.g. slices of numa nodes, cores, processors ..);
	// see handle.SortedTopology()
	if topology1.Architecture != topology2.Architecture {
		return fmt.Errorf("the architecture is different: %v vs %v", topology1.Architecture, topology2.Architecture)
	}

	if len(topology1.Nodes) != len(topology2.Nodes) {
		return fmt.Errorf("the number of NUMA nodes differ: %v vs %v", len(topology1.Nodes), len(topology2.Nodes))
	}

	for i, node1 := range topology1.Nodes {
		node2 := topology2.Nodes[i]
		if node1.ID != node2.ID {
			return fmt.Errorf("the NUMA node ids differ: %v vs %v", node1.ID, node2.ID)
		}

		cores1 := node1.Cores
		cores2 := node2.Cores
		if len(cores1) != len(cores2) {
			return fmt.Errorf("the number of CPU cores in NUMA node %d differ: %v vs %v",
				node1.ID, len(topology1.Nodes), len(topology2.Nodes))
		}

		for j, core1 := range cores1 {
			if core1.ID != cores2[j].ID {
				// it was learned that core numbering can have different schemes even with
				// a system from the same vendor. One case was observed on Intel Xeon Gold 6438N with 0-127
				// online CPUs distributed across 2 sockets, 32 cores per socket and 2 threads per core.
				// The numbering pattern depends on the settings of the hardware, the software and the
				// firmware (BIOS).While core IDs may vary nodes can still be considered having same NUMA
				// topology taking into account that core scope is on the single NUMA. In other words, as long
				// as the NUMA cells have same logical processors' count and IDs and same threads' number,
				// core ID equality is treated as best effort. That is because when scheduling workloads,
				// we care about the logical processors ids and their location on the NUMAs.
				Alert("the CPU core ids in NUMA node %d differ: %d vs %d", node1.ID, core1.ID, cores2[j].ID)
				tols[toleration.DifferentCoreIDs] = true
			}
			if core1.NumThreads != cores2[j].NumThreads {
				return fmt.Errorf("number of threads for CPU %d in NUMA node %d differs: %d vs %d", core1.ID, node1.ID, core1.NumThreads, cores2[j].NumThreads)
			}
			if !reflect.DeepEqual(core1.LogicalProcessors, cores2[j].LogicalProcessors) {
				return fmt.Errorf("logical processors for CPU %d in NUMA node %d differs: %d vs %d", core1.ID, node1.ID, core1.LogicalProcessors, cores2[j].LogicalProcessors)
			}
		}
	}
	return nil
}

// GetAdditionalKernelArgs returns a set of kernel parameters based on configuration
func GetAdditionalKernelArgs(disableHT bool) []string {
	var kernelArgs []string
	if disableHT {
		kernelArgs = append(kernelArgs, noSMTKernelArg)
	}
	sort.Strings(kernelArgs)
	Alert("Additional Kernel Args based on configuration: %v", kernelArgs)
	return kernelArgs
}

func updateExtendedCPUInfo(extCpuInfo *extendedCPUInfo, used cpuset.CPUSet, disableHT, htEnabled bool) (*extendedCPUInfo, error) {
	retCpuInfo := &cpu.Info{
		TotalCores:   0,
		TotalThreads: 0,
	}

	ret := &extendedCPUInfo{
		CpuInfo:                  retCpuInfo,
		NumLogicalProcessorsUsed: make(map[int]int, len(extCpuInfo.NumLogicalProcessorsUsed)),
		LogicalProcessorsUsed:    make(map[int]struct{}),
	}
	for k, v := range extCpuInfo.NumLogicalProcessorsUsed {
		ret.NumLogicalProcessorsUsed[k] = v
	}
	for k, v := range extCpuInfo.LogicalProcessorsUsed {
		ret.LogicalProcessorsUsed[k] = v
	}

	cpuInfo := extCpuInfo.CpuInfo
	for _, socket := range cpuInfo.Processors {
		s := &cpu.Processor{
			ID:           socket.ID,
			Vendor:       socket.Vendor,
			Model:        socket.Model,
			Capabilities: socket.Capabilities,
			NumCores:     0,
			NumThreads:   0,
		}

		for _, core := range socket.Cores {
			c := &cpu.ProcessorCore{
				ID:         core.ID,
				NumThreads: 0,
			}

			for index, lp := range core.LogicalProcessors {
				if used.Contains(lp) {
					if val, ok := ret.NumLogicalProcessorsUsed[socket.ID]; ok {
						ret.NumLogicalProcessorsUsed[socket.ID] = val + 1
					} else {
						ret.NumLogicalProcessorsUsed[socket.ID] = 1
					}
					ret.LogicalProcessorsUsed[lp] = struct{}{}
				}
				if htEnabled && disableHT {
					if index == 0 {
						c.LogicalProcessors = append(c.LogicalProcessors, lp)
						c.NumThreads++
					}
				} else {
					c.LogicalProcessors = append(c.LogicalProcessors, lp)
					c.NumThreads++
				}
			}

			if c.NumThreads > 0 {
				s.NumThreads += c.NumThreads
				s.NumCores++
				s.Cores = append(s.Cores, c)
			}
		}

		if s.NumCores > 0 {
			retCpuInfo.TotalThreads += s.NumThreads
			retCpuInfo.TotalCores += s.NumCores
			retCpuInfo.Processors = append(retCpuInfo.Processors, s)
		}
	}

	return ret, nil
}

func IsLogicalProcessorUsed(extCPUInfo *extendedCPUInfo, logicalProcessor int) bool {
	_, ok := extCPUInfo.LogicalProcessorsUsed[logicalProcessor]
	return ok
}

// IsExternalControlPlaneCluster return whether the control plane is running on externally outside the cluster
func IsExternalControlPlaneCluster(mustGatherDirPath string) (bool, error) {
	infraPath := path.Join(ClusterScopedResources, configOCPInfra, "cluster.yaml")
	fullInfraPath, err := getMustGatherFullPaths(mustGatherDirPath, infraPath)
	if fullInfraPath == "" || err != nil {
		return false, fmt.Errorf("failed to get Infrastructure object from must gather directory path: %s; %w", mustGatherDirPath, err)
	}
	f, err := os.Open(fullInfraPath)
	if err != nil {
		return false, fmt.Errorf("failed to open file %s; %w", fullInfraPath, err)
	}
	infra := &configv1.Infrastructure{}
	dec := k8syaml.NewYAMLOrJSONDecoder(f, 1024)
	if err := dec.Decode(infra); err != nil {
		return false, fmt.Errorf("failed to Decode file %s; %w", fullInfraPath, err)
	}
	return infra.Status.ControlPlaneTopology == configv1.ExternalTopologyMode, nil
}
