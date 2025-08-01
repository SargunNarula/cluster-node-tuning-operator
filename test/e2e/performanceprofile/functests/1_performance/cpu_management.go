package __performance

import (
	"context"
	"encoding/json"
	"fmt"
	"os"
	"regexp"
	"strconv"
	"strings"
	"time"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/types"
	kubeletconfigv1beta1 "k8s.io/kubelet/config/v1beta1"
	"k8s.io/utils/cpuset"
	"k8s.io/utils/ptr"
	"sigs.k8s.io/controller-runtime/pkg/client"

	. "github.com/onsi/ginkgo/v2"
	. "github.com/onsi/gomega"

	performancev2 "github.com/openshift/cluster-node-tuning-operator/pkg/apis/performanceprofile/v2"
	"github.com/openshift/cluster-node-tuning-operator/pkg/performanceprofile/controller/performanceprofile/components"
	"github.com/openshift/cluster-node-tuning-operator/pkg/performanceprofile/utils/schedstat"
	manifestsutil "github.com/openshift/cluster-node-tuning-operator/pkg/util"
	testutils "github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/cgroup"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/cgroup/controller"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/cgroup/runtime"
	testclient "github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/client"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/cluster"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/deployments"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/discovery"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/events"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/images"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/label"
	testlog "github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/log"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/nodes"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/pods"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/profiles"
)

var workerRTNode *corev1.Node
var profile *performancev2.PerformanceProfile

const restartCooldownTime = 1 * time.Minute
const cgroupRoot string = "/sys/fs/cgroup"

type CPUVals struct {
	CPUs string `json:"cpus"`
}

type CPUResources struct {
	CPU CPUVals `json:"cpu"`
}

type LinuxResources struct {
	Resources CPUResources `json:"resources"`
}

type Process struct {
	Args []string `json:"args"`
}

type Annotations struct {
	ContainerName string `json:"io.kubernetes.container.name"`
	PodName       string `json:"io.kubernetes.pod.name"`
}

type ContainerConfig struct {
	Process     Process        `json:"process"`
	Hostname    string         `json:"hostname"`
	Annotations Annotations    `json:"annotations"`
	Linux       LinuxResources `json:"linux"`
}

var _ = Describe("[rfe_id:27363][performance] CPU Management", Ordered, func() {
	var (
		balanceIsolated          bool
		reservedCPU, isolatedCPU string
		listReservedCPU          []int
		reservedCPUSet           cpuset.CPUSet
		onlineCPUSet             cpuset.CPUSet
		isolatedCPUSet           cpuset.CPUSet
		err                      error
		smtLevel                 int
		ctx                      context.Context = context.Background()
		getter                   cgroup.ControllersGetter
		cgroupV2                 bool
	)

	testutils.CustomBeforeAll(func() {
		isSNO, err := cluster.IsSingleNode()
		Expect(err).ToNot(HaveOccurred())
		RunningOnSingleNode = isSNO
		workerRTNodes, err := nodes.GetByLabels(testutils.NodeSelectorLabels)
		Expect(err).ToNot(HaveOccurred())
		workerRTNodes, err = nodes.MatchingOptionalSelector(workerRTNodes)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("error looking for the optional selector: %v", err))
		Expect(workerRTNodes).ToNot(BeEmpty())
		workerRTNode = &workerRTNodes[0]

		onlineCPUSet, err = nodes.GetOnlineCPUsSet(ctx, workerRTNode)
		Expect(err).ToNot(HaveOccurred())
		cpuID := onlineCPUSet.UnsortedList()[0]
		smtLevel = nodes.GetSMTLevel(ctx, cpuID, workerRTNode)
		getter, err = cgroup.BuildGetter(ctx, testclient.DataPlaneClient, testclient.K8sClient)
		Expect(err).ToNot(HaveOccurred())
		cgroupV2, err = cgroup.IsVersion2(ctx, testclient.DataPlaneClient)
		Expect(err).ToNot(HaveOccurred())

	})

	BeforeEach(func() {
		if discovery.Enabled() && testutils.ProfileNotFound {
			Skip("Discovery mode enabled, performance profile not found")
		}
		profile, err = profiles.GetByNodeLabels(testutils.NodeSelectorLabels)
		Expect(err).ToNot(HaveOccurred())

		cpus, err := cpuSpecToString(profile.Spec.CPU)
		Expect(err).ToNot(HaveOccurred(), "failed to parse cpu %v spec to string", cpus)
		By(fmt.Sprintf("Checking the profile %s with cpus %s", profile.Name, cpus))
		balanceIsolated = true
		if profile.Spec.CPU.BalanceIsolated != nil {
			balanceIsolated = *profile.Spec.CPU.BalanceIsolated
		}

		Expect(profile.Spec.CPU.Isolated).NotTo(BeNil())
		isolatedCPU = string(*profile.Spec.CPU.Isolated)
		isolatedCPUSet, err = cpuset.Parse(isolatedCPU)
		Expect(err).ToNot(HaveOccurred())

		Expect(profile.Spec.CPU.Reserved).NotTo(BeNil())
		reservedCPU = string(*profile.Spec.CPU.Reserved)
		reservedCPUSet, err = cpuset.Parse(reservedCPU)
		Expect(err).ToNot(HaveOccurred())
		listReservedCPU = reservedCPUSet.List()

		onlineCPUSet, err = nodes.GetOnlineCPUsSet(context.TODO(), workerRTNode)
		Expect(err).ToNot(HaveOccurred())
	})

	Describe("Verification of configuration on the worker node", Label(string(label.Tier0)), func() {
		It("[test_id:28528][crit:high][vendor:cnf-qe@redhat.com][level:acceptance] Verify CPU reservation on the node", func() {
			By(fmt.Sprintf("Allocatable CPU should be less than capacity by %d", len(listReservedCPU)))
			capacityCPU, _ := workerRTNode.Status.Capacity.Cpu().AsInt64()
			allocatableCPU, _ := workerRTNode.Status.Allocatable.Cpu().AsInt64()
			differenceCPUGot := capacityCPU - allocatableCPU
			differenceCPUExpected := int64(len(listReservedCPU))
			Expect(differenceCPUGot).To(Equal(differenceCPUExpected), "Allocatable CPU %d should be less than capacity %d by %d; got %d instead", allocatableCPU, capacityCPU, differenceCPUExpected, differenceCPUGot)
		})

		It("[test_id:37862][crit:high][vendor:cnf-qe@redhat.com][level:acceptance] Verify CPU affinity mask, CPU reservation and CPU isolation on worker node", func() {
			By("checking isolated CPU")
			cmd := []string{"cat", "/sys/devices/system/cpu/isolated"}
			out, err := nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
			Expect(err).ToNot(HaveOccurred())
			sysIsolatedCpus := testutils.ToString(out)
			if balanceIsolated {
				Expect(sysIsolatedCpus).To(BeEmpty())
			} else {
				Expect(sysIsolatedCpus).To(Equal(isolatedCPU))
			}

			By("checking reserved CPU in kubelet config file")
			cmd = []string{"cat", "/rootfs/etc/kubernetes/kubelet.conf"}
			out, err = nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
			Expect(err).ToNot(HaveOccurred(), "failed to cat kubelet.conf")
			conf := testutils.ToString(out)
			obj, err := manifestsutil.DeserializeObjectFromData([]byte(conf), kubeletconfigv1beta1.AddToScheme)
			Expect(err).ToNot(HaveOccurred())
			kc, ok := obj.(*kubeletconfigv1beta1.KubeletConfiguration)
			Expect(ok).To(BeTrue(), "wrong type %T", obj)
			Expect(kc.ReservedSystemCPUs).To(Equal(reservedCPU))

			By("checking CPU affinity mask for kernel scheduler")
			cmd = []string{"/bin/bash", "-c", "taskset -pc 1"}
			out, err = nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
			Expect(err).ToNot(HaveOccurred(), "failed to execute taskset")
			sched := testutils.ToString(out)
			mask := strings.SplitAfter(sched, " ")
			maskSet, err := cpuset.Parse(mask[len(mask)-1])
			Expect(err).ToNot(HaveOccurred())

			Expect(reservedCPUSet.IsSubsetOf(maskSet)).To(Equal(true), fmt.Sprintf("The init process (pid 1) should have cpu affinity: %s", reservedCPU))
		})

		It("[test_id:34358] Verify rcu_nocbs kernel argument on the node", func() {
			By("checking that cmdline contains rcu_nocbs with right value")
			cmd := []string{"cat", "/proc/cmdline"}
			out, err := nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
			Expect(err).ToNot(HaveOccurred())
			cmdline := testutils.ToString(out)
			re := regexp.MustCompile(`rcu_nocbs=\S+`)
			rcuNocbsArgument := re.FindString(cmdline)
			Expect(rcuNocbsArgument).To(ContainSubstring("rcu_nocbs="))
			rcuNocbsCpu := strings.Split(rcuNocbsArgument, "=")[1]
			rcuNocbsCPUSet, err := cpuset.Parse(rcuNocbsCpu)
			Expect(err).ToNot(HaveOccurred())
			Expect(rcuNocbsCPUSet.Equals(isolatedCPUSet)).To(BeTrue(), "rcu_nocbs CPUs do not match isolated CPUs")

			By("checking that new rcuo processes are running on non_isolated cpu")
			cmd = []string{"pgrep", "rcuo"}
			out, err = nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
			Expect(err).ToNot(HaveOccurred())
			rcuoList := testutils.ToString(out)
			for _, rcuo := range strings.Split(rcuoList, "\n") {
				// check cpu affinity mask
				cmd = []string{"/bin/bash", "-c", fmt.Sprintf("taskset -pc %s", rcuo)}
				out, err := nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
				Expect(err).ToNot(HaveOccurred())
				taskset := testutils.ToString(out)
				mask := strings.SplitAfter(taskset, " ")
				maskSet, err := cpuset.Parse(mask[len(mask)-1])
				Expect(err).ToNot(HaveOccurred())
				Expect(reservedCPUSet.IsSubsetOf(maskSet)).To(Equal(true), "The process should have cpu affinity: %s", reservedCPU)
			}
		})

	})

	Describe("Verification of cpu manager functionality", Label(string(label.Tier0)), func() {
		var testpod *corev1.Pod
		var discoveryFailed bool

		testutils.CustomBeforeAll(func() {
			discoveryFailed = false
			if discovery.Enabled() {
				profile, err := profiles.GetByNodeLabels(testutils.NodeSelectorLabels)
				Expect(err).ToNot(HaveOccurred())
				isolatedCPU = string(*profile.Spec.CPU.Isolated)
			}
		})

		BeforeEach(func() {
			if discoveryFailed {
				Skip("Skipping tests since there are insufficant isolated cores to create a stress pod")
			}
		})

		AfterEach(func() {
			deleteTestPod(context.TODO(), testpod)
		})

		DescribeTable("Verify CPU usage by stress PODs", func(ctx context.Context, guaranteed bool) {
			cpuID := onlineCPUSet.UnsortedList()[0]
			smtLevel := nodes.GetSMTLevel(ctx, cpuID, workerRTNode)
			if smtLevel < 2 {
				Skip(fmt.Sprintf("designated worker node %q has SMT level %d - minimum required 2", workerRTNode.Name, smtLevel))
			}

			// note must be a multiple of the smtLevel. Pick the minimum to maximize the chances to run on CI
			cpuRequest := smtLevel
			testpod = getStressPod(workerRTNode.Name, cpuRequest)
			testpod.Namespace = testutils.NamespaceTesting

			// the worker node on which the pod will be scheduled has other pods already scheduled on it, and these also use a
			// portion of the node's isolated cpus, and scheduling a pod requesting all the isolated cpus on the node (hence =)
			// would fail because there is already a base cpu load on the node
			if cpuRequest >= isolatedCPUSet.Size() {
				Skip(fmt.Sprintf("cpus request %d is greater than the available on the node as the isolated cpus are %d", cpuRequest, isolatedCPUSet.Size()))
			}

			listCPU := onlineCPUSet.List()
			expectedQos := corev1.PodQOSBurstable

			if guaranteed {
				listCPU = onlineCPUSet.Difference(reservedCPUSet).List()
				expectedQos = corev1.PodQOSGuaranteed
				promotePodToGuaranteed(testpod)
			} else if !balanceIsolated {
				// when balanceIsolated is False - non-guaranteed pod should run on reserved cpu
				listCPU = listReservedCPU
			}

			By(fmt.Sprintf("create a %s QoS stress pod requesting %d cpus", expectedQos, cpuRequest))
			var err error
			err = testclient.DataPlaneClient.Create(ctx, testpod)
			Expect(err).ToNot(HaveOccurred())

			testpod, err = pods.WaitForCondition(ctx, client.ObjectKeyFromObject(testpod), corev1.PodReady, corev1.ConditionTrue, 10*time.Minute)
			logEventsForPod(testpod)
			Expect(err).ToNot(HaveOccurred())

			updatedPod := &corev1.Pod{}
			err = testclient.DataPlaneClient.Get(ctx, client.ObjectKeyFromObject(testpod), updatedPod)
			Expect(err).ToNot(HaveOccurred())
			Expect(updatedPod.Status.QOSClass).To(Equal(expectedQos),
				"unexpected QoS Class for %s/%s: %s (looking for %s)",
				updatedPod.Namespace, updatedPod.Name, updatedPod.Status.QOSClass, expectedQos)

			out, err := nodes.ExecCommand(ctx,
				workerRTNode,
				[]string{"/bin/bash", "-c", "ps -o psr $(pgrep -n stress) | tail -1"},
			)
			Expect(err).ToNot(HaveOccurred(), "failed to get cpu of stress process")
			output := testutils.ToString(out)
			cpu, err := strconv.Atoi(strings.Trim(output, " "))
			Expect(err).ToNot(HaveOccurred())

			Expect(cpu).To(BeElementOf(listCPU))
		},
			Entry("[test_id:37860] Non-guaranteed POD can work on any CPU", context.TODO(), false),
			Entry("[test_id:27492] Guaranteed POD should work on isolated cpu", context.TODO(), true),
		)
	})

	Describe("Verification of cpu_manager_state file", Label(string(label.Tier0)), func() {
		var testpod *corev1.Pod
		BeforeEach(func() {
			testpod = pods.GetTestPod()
			cpuRequest := 2
			testpod.Namespace = testutils.NamespaceTesting
			testpod.Spec.NodeSelector = map[string]string{testutils.LabelHostname: workerRTNode.Name}
			testpod.Spec.Containers[0].Resources = corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Mi"),
					corev1.ResourceCPU:    resource.MustParse(fmt.Sprintf("%d", cpuRequest)),
				},
			}

			if cpuRequest >= isolatedCPUSet.Size() {
				Skip(fmt.Sprintf("cpus request %d is greater than the available on the node as the isolated cpus are %d", cpuRequest, isolatedCPUSet.Size()))
			}

			err := testclient.DataPlaneClient.Create(context.TODO(), testpod)
			Expect(err).ToNot(HaveOccurred())
			testpod, err = pods.WaitForCondition(context.TODO(), client.ObjectKeyFromObject(testpod), corev1.PodReady, corev1.ConditionTrue, 10*time.Minute)
			logEventsForPod(testpod)
			Expect(err).ToNot(HaveOccurred())
		})
		AfterEach(func() {
			deleteTestPod(context.TODO(), testpod)
		})
		When("kubelet is restart", func() {
			It("[test_id: 73501] defaultCpuset should not change", func() {
				testutils.KnownIssueJira("OCPBUGS-43280")
				By("fetch Default cpu set from cpu manager state file before restart")
				cpuManagerCpusetBeforeRestart, err := nodes.CpuManagerCpuSet(ctx, workerRTNode)
				Expect(err).ToNot(HaveOccurred())
				testlog.Infof("pre kubelet restart default cpuset: %v", cpuManagerCpusetBeforeRestart.String())

				By("capturing test pod state before restart")
				originalPodUID := testpod.UID
				testlog.Infof("pre kubelet restart pod UID: %v", originalPodUID)

				kubeletRestartCmd := []string{
					"chroot",
					"/rootfs",
					"/bin/bash",
					"-c",
					"systemctl restart kubelet",
				}

				_, _ = nodes.ExecCommand(ctx, workerRTNode, kubeletRestartCmd)
				nodes.WaitForReadyOrFail("post kubele restart", workerRTNode.Name, 20*time.Minute, 3*time.Second)
				// giving kubelet more time to stabilize and initialize itself before
				testlog.Infof("post restart: entering cooldown time: %v", restartCooldownTime)
				time.Sleep(restartCooldownTime)

				testlog.Infof("post restart: finished cooldown time: %v", restartCooldownTime)

				By("verify test pod comes back after kubelet restart")
				Eventually(func() error {
					var updatedPod corev1.Pod
					err := testclient.DataPlaneClient.Get(ctx, client.ObjectKeyFromObject(testpod), &updatedPod)
					if err != nil {
						return fmt.Errorf("failed to get pod after restart: %v", err)
					}

					// Verify it's the same pod (same UID)
					if updatedPod.UID != originalPodUID {
						return fmt.Errorf("pod UID changed after restart: original=%v, current=%v", originalPodUID, updatedPod.UID)
					}

					// Verify pod is ready
					if updatedPod.Status.Phase != corev1.PodRunning {
						return fmt.Errorf("pod is not running after restart: phase=%v", updatedPod.Status.Phase)
					}
					// Check pod ready condition
					for _, condition := range updatedPod.Status.Conditions {
						if condition.Type == corev1.PodReady && condition.Status != corev1.ConditionTrue {
							return fmt.Errorf("Pod condition is not in Ready state after kubelet restart: reason: %v, message: %v", condition.Reason, condition.Message)

						}
					}
					return nil
				}).WithTimeout(5*time.Minute).WithPolling(10*time.Second).Should(Succeed(), "test pod should come back after kubelet restart")

				By("fetch Default cpuset from cpu manager state after restart")
				cpuManagerCpusetAfterRestart, err := nodes.CpuManagerCpuSet(ctx, workerRTNode)
				Expect(err).ToNot(HaveOccurred())
				Expect(cpuManagerCpusetBeforeRestart).To(Equal(cpuManagerCpusetAfterRestart))
			})
		})
	})

	Describe("Verification that IRQ load balance can be disabled per POD", Label(string(label.Tier0)), func() {
		var smtLevel int
		var testpod *corev1.Pod

		BeforeEach(func() {
			Skip("part of interrupts does not support CPU affinity change because of underlying hardware")

			if profile.Spec.GloballyDisableIrqLoadBalancing != nil && *profile.Spec.GloballyDisableIrqLoadBalancing {
				Skip("IRQ load balance should be enabled (GloballyDisableIrqLoadBalancing=false), skipping test")
			}

			cpuID := onlineCPUSet.UnsortedList()[0]
			smtLevel = nodes.GetSMTLevel(context.TODO(), cpuID, workerRTNode)
		})

		AfterEach(func() {
			if testpod != nil {
				deleteTestPod(context.TODO(), testpod)
			}
		})

		It("[test_id:36364] should disable IRQ balance for CPU where POD is running", func() {
			By("checking default smp affinity is equal to all active CPUs")
			defaultSmpAffinitySet, err := nodes.GetDefaultSmpAffinitySet(context.TODO(), workerRTNode)
			Expect(err).ToNot(HaveOccurred())

			onlineCPUsSet, err := nodes.GetOnlineCPUsSet(context.TODO(), workerRTNode)
			Expect(err).ToNot(HaveOccurred())

			Expect(onlineCPUsSet.IsSubsetOf(defaultSmpAffinitySet)).To(BeTrue(), "All online CPUs %s should be subset of default SMP affinity %s", onlineCPUsSet, defaultSmpAffinitySet)

			By("Running pod with annotations that disable specific CPU from IRQ balancer")
			annotations := map[string]string{
				"irq-load-balancing.crio.io": "disable",
				"cpu-quota.crio.io":          "disable",
			}
			testpod = getTestPodWithAnnotations(annotations, smtLevel)

			err = testclient.DataPlaneClient.Create(context.TODO(), testpod)
			Expect(err).ToNot(HaveOccurred())
			testpod, err = pods.WaitForCondition(context.TODO(), client.ObjectKeyFromObject(testpod), corev1.PodReady, corev1.ConditionTrue, 10*time.Minute)
			logEventsForPod(testpod)
			Expect(err).ToNot(HaveOccurred())

			By("Checking that the default smp affinity mask was updated and CPU (where POD is running) isolated")
			defaultSmpAffinitySet, err = nodes.GetDefaultSmpAffinitySet(context.TODO(), workerRTNode)
			Expect(err).ToNot(HaveOccurred())

			getPsr := []string{"/bin/bash", "-c", "grep Cpus_allowed_list /proc/self/status | awk '{print $2}'"}
			psr, err := pods.WaitForPodOutput(context.TODO(), testclient.K8sClient, testpod, getPsr)
			Expect(err).ToNot(HaveOccurred())
			psrSet, err := cpuset.Parse(strings.Trim(string(psr), "\n"))
			Expect(err).ToNot(HaveOccurred())

			Expect(psrSet.IsSubsetOf(defaultSmpAffinitySet)).To(BeFalse(), fmt.Sprintf("Default SMP affinity should not contain isolated CPU %s", psr))

			By("Checking that there are no any active IRQ on isolated CPU")
			// It may takes some time for the system to reschedule active IRQs
			Eventually(func() bool {
				getActiveIrq := []string{"/bin/bash", "-c", "for n in $(find /proc/irq/ -name smp_affinity_list); do echo $(cat $n); done"}
				out, err := nodes.ExecCommand(context.TODO(), workerRTNode, getActiveIrq)
				Expect(err).ToNot(HaveOccurred())
				activeIrq := testutils.ToString(out)
				Expect(activeIrq).ToNot(BeEmpty())
				for _, irq := range strings.Split(activeIrq, "\n") {
					irqAffinity, err := cpuset.Parse(irq)
					Expect(err).ToNot(HaveOccurred())
					if !irqAffinity.Equals(onlineCPUsSet) && psrSet.IsSubsetOf(irqAffinity) {
						return false
					}
				}
				return true
			}).WithTimeout(cluster.ComputeTestTimeout(30*time.Second, RunningOnSingleNode)).WithPolling(5*time.Second).Should(BeTrue(),
				fmt.Sprintf("IRQ still active on CPU%s", psr))

			By("Checking that after removing POD default smp affinity is returned back to all active CPUs")
			deleteTestPod(context.TODO(), testpod)
			defaultSmpAffinitySet, err = nodes.GetDefaultSmpAffinitySet(context.TODO(), workerRTNode)
			Expect(err).ToNot(HaveOccurred())

			Expect(onlineCPUsSet.IsSubsetOf(defaultSmpAffinitySet)).To(BeTrue(), "All online CPUs %s should be subset of default SMP affinity %s", onlineCPUsSet, defaultSmpAffinitySet)
		})
	})

	When("reserved CPUs specified", Label(string(label.Tier0)), func() {
		var testpod *corev1.Pod

		BeforeEach(func() {
			testpod = pods.GetTestPod()
			testpod.Namespace = testutils.NamespaceTesting
			testpod.Spec.NodeSelector = map[string]string{testutils.LabelHostname: workerRTNode.Name}
			testpod.Spec.ShareProcessNamespace = ptr.To(true)

			err := testclient.DataPlaneClient.Create(context.TODO(), testpod)
			Expect(err).ToNot(HaveOccurred())

			testpod, err = pods.WaitForCondition(context.TODO(), client.ObjectKeyFromObject(testpod), corev1.PodReady, corev1.ConditionTrue, 10*time.Minute)
			logEventsForPod(testpod)
			Expect(err).ToNot(HaveOccurred())
		})

		It("[test_id:49147] should run infra containers on reserved CPUs", func() {
			var cpusetPath string
			// find used because that crictl does not show infra containers, `runc list` shows them
			// but you will need somehow to find infra containers ID's
			podUID := strings.Replace(string(testpod.UID), "-", "_", -1)
			podCgroup := ""
			if cgroupV2 {
				cpusetPath = "/rootfs/sys/fs/cgroup/kubepods.slice"
			} else {
				cpusetPath = "/rootfs/sys/fs/cgroup/cpuset"
			}

			Eventually(func() string {
				cmd := []string{"/bin/bash", "-c", fmt.Sprintf("find %s -name *%s*", cpusetPath, podUID)}
				out, err := nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
				Expect(err).ToNot(HaveOccurred())
				podCgroup = testutils.ToString(out)
				return podCgroup
			}, cluster.ComputeTestTimeout(30*time.Second, RunningOnSingleNode), 5*time.Second).ShouldNot(BeEmpty(),
				fmt.Sprintf("cannot find cgroup for pod %q", podUID))

			containersCgroups := ""
			Eventually(func() string {
				cmd := []string{"/bin/bash", "-c", fmt.Sprintf("find %s -name crio-*", podCgroup)}
				out, err := nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
				Expect(err).ToNot(HaveOccurred())
				containersCgroups = testutils.ToString(out)
				return containersCgroups
			}, cluster.ComputeTestTimeout(30*time.Second, RunningOnSingleNode), 5*time.Second).ShouldNot(BeEmpty(),
				fmt.Sprintf("cannot find containers cgroups from pod cgroup %q", podCgroup))

			containerID, err := pods.GetContainerIDByName(testpod, "test")
			Expect(err).ToNot(HaveOccurred())

			containersCgroups = strings.Trim(containersCgroups, "\n")
			containersCgroupsDirs := strings.Split(containersCgroups, "\n")

			for _, dir := range containersCgroupsDirs {
				// skip application container cgroup
				// skip conmon containers
				if strings.Contains(dir, containerID) || strings.Contains(dir, "conmon") {
					continue
				}

				By("Checking what CPU the infra container is using")
				cmd := []string{"/bin/bash", "-c", fmt.Sprintf("cat %s/cpuset.cpus", dir)}
				out, err := nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
				Expect(err).ToNot(HaveOccurred())
				output := testutils.ToString(out)
				cpus, err := cpuset.Parse(output)
				Expect(err).ToNot(HaveOccurred())

				Expect(cpus.List()).To(Equal(reservedCPUSet.List()))
			}
		})
	})

	When("strict NUMA aligment is requested", Label(string(label.Tier0)), func() {
		var testpod *corev1.Pod

		BeforeEach(func() {
			if profile.Spec.NUMA == nil || profile.Spec.NUMA.TopologyPolicy == nil {
				Skip("Topology Manager Policy is not configured")
			}
			tmPolicy := *profile.Spec.NUMA.TopologyPolicy
			if tmPolicy != "single-numa-node" {
				Skip("Topology Manager Policy is not Single NUMA Node")
			}
		})

		AfterEach(func() {
			if testpod == nil {
				return
			}
			deleteTestPod(context.TODO(), testpod)
		})

		It("[test_id:49149] should reject pods which request integral CPUs not aligned with machine SMT level", func() {
			// also covers Hyper-thread aware sheduling [test_id:46545] Odd number of isolated CPU threads
			// any random existing cpu is fine
			cpuID := onlineCPUSet.UnsortedList()[0]
			smtLevel := nodes.GetSMTLevel(context.TODO(), cpuID, workerRTNode)
			if smtLevel < 2 {
				Skip(fmt.Sprintf("designated worker node %q has SMT level %d - minimum required 2", workerRTNode.Name, smtLevel))
			}

			cpuCount := 1 // must be intentionally < than the smtLevel to trigger the kubelet validation
			testpod = promotePodToGuaranteed(getStressPod(workerRTNode.Name, cpuCount))
			testpod.Namespace = testutils.NamespaceTesting

			err := testclient.DataPlaneClient.Create(context.TODO(), testpod)
			Expect(err).ToNot(HaveOccurred())

			currentPod, err := pods.WaitForPredicate(context.TODO(), client.ObjectKeyFromObject(testpod), 10*time.Minute, func(pod *corev1.Pod) (bool, error) {
				if pod.Status.Phase != corev1.PodPending {
					return true, nil
				}
				return false, nil
			})
			Expect(err).ToNot(HaveOccurred(), "expected the pod to keep pending, but its current phase is %s", currentPod.Status.Phase)

			updatedPod := &corev1.Pod{}
			err = testclient.DataPlaneClient.Get(context.TODO(), client.ObjectKeyFromObject(testpod), updatedPod)
			Expect(err).ToNot(HaveOccurred())

			Expect(updatedPod.Status.Phase).To(Equal(corev1.PodFailed), "pod %s not failed: %v", updatedPod.Name, updatedPod.Status)
			Expect(isSMTAlignmentError(updatedPod)).To(BeTrue(), "pod %s failed for wrong reason: %q", updatedPod.Name, updatedPod.Status.Reason)
		})
	})
	Describe("Hyper-thread aware scheduling for guaranteed pods", Label(string(label.Tier1)), func() {
		var testpod *corev1.Pod

		BeforeEach(func() {
			if profile.Spec.NUMA == nil || profile.Spec.NUMA.TopologyPolicy == nil {
				Skip("Topology Manager Policy is not configured")
			}
			tmPolicy := *profile.Spec.NUMA.TopologyPolicy
			if tmPolicy != "single-numa-node" {
				Skip("Topology Manager Policy is not Single NUMA Node")
			}
		})

		AfterEach(func() {
			if testpod == nil {
				return
			}
			deleteTestPod(context.TODO(), testpod)
		})

		DescribeTable("Verify Hyper-Thread aware scheduling for guaranteed pods",
			func(ctx context.Context, htDisabled bool, snoCluster bool, snoWP bool) {
				// Check for SMT enabled
				// any random existing cpu is fine
				cpuCounts := make([]int, 0, 2)
				//var testpod *corev1.Pod
				//var err error

				// Check for SMT enabled
				// any random existing cpu is fine
				cpuID := onlineCPUSet.UnsortedList()[0]
				smtLevel := nodes.GetSMTLevel(ctx, cpuID, workerRTNode)
				hasWP := checkForWorkloadPartitioning(ctx)

				// Following checks are required to map test_id scenario correctly to the type of node under test
				if snoCluster && !RunningOnSingleNode {
					Skip("Requires SNO cluster")
				}
				if !snoCluster && RunningOnSingleNode {
					Skip("Requires Non-SNO cluster")
				}
				if (smtLevel < 2) && !htDisabled {
					Skip(fmt.Sprintf("designated worker node %q has SMT level %d - minimum required 2", workerRTNode.Name, smtLevel))
				}
				if (smtLevel > 1) && htDisabled {
					Skip(fmt.Sprintf("designated worker node %q has SMT level %d - requires exactly 1", workerRTNode.Name, smtLevel))
				}

				if (snoCluster && snoWP) && !hasWP {
					Skip("Requires SNO cluster with Workload Partitioning enabled")
				}
				if (snoCluster && !snoWP) && hasWP {
					Skip("Requires SNO cluster without Workload Partitioning enabled")
				}
				cpuCounts = append(cpuCounts, 2)
				if htDisabled {
					cpuCounts = append(cpuCounts, 1)
				}

				for _, cpuCount := range cpuCounts {
					testpod = startHTtestPod(ctx, cpuCount)
					Expect(checkPodHTSiblings(ctx, testpod)).To(BeTrue(), "Pod cpu set does not map to host cpu sibling pairs")
					By("Deleting test pod...")
					deleteTestPod(ctx, testpod)
				}
			},

			Entry("[test_id:46959] Number of CPU requests as multiple of SMT count allowed when HT enabled", context.TODO(), false, false, false),
			Entry("[test_id:46544] Odd number of CPU requests allowed when HT disabled", context.TODO(), true, false, false),
			Entry("[test_id:46538] HT aware scheduling on SNO cluster", context.TODO(), false, true, false),
			Entry("[test_id:46539] HT aware scheduling on SNO cluster and Workload Partitioning enabled", context.TODO(), false, true, true),
		)

	})
	// Automates OCPBUGS-34812: cgroupsv2: failed to write on cpuset.cpus.exclusive
	Context("Cgroupsv2", func() {
		It("[test_id:75327] cpus from deleted cgroup can be reassigned to new cgroup", Label(string(label.Tier0)), func() {

			// we need system with more than 10 cpus to execute this test
			if len(onlineCPUSet.List()) < 10 {
				Skip("Requires system with  more than 10 cpus")
			}
			if !cgroupV2 {
				Skip("Requires CgroupV2")
			}
			// create deployment with 2 replicas and each pod having cpu load balancing disabled
			// and runtime class. This is required as the cpu id's used by the container are
			// written to cpuset.cpus.exclusive
			const DeploymentName = "test-deployment"
			var numberofReplicas int32 = 2
			var dp *appsv1.Deployment
			annotations := map[string]string{
				"cpu-load-balancing.crio.io": "disable",
			}
			p := pods.GetTestPod()
			p.Spec.NodeSelector = testutils.NodeSelectorLabels
			p.ObjectMeta = metav1.ObjectMeta{
				Labels: map[string]string{
					"app": DeploymentName,
				},
			}
			runtimeClass := components.GetComponentName(profile.Name, components.ComponentNamePrefix)
			p.Spec.RuntimeClassName = &runtimeClass
			p.Spec.Containers[0].Image = images.Test()
			p.Spec.Containers[0].Resources = corev1.ResourceRequirements{
				Limits: corev1.ResourceList{
					corev1.ResourceMemory: resource.MustParse("200Mi"),
					corev1.ResourceCPU:    resource.MustParse("2"),
				},
			}

			// Adding a unique label to the deployment
			deploymentLabels := map[string]string{
				"app": DeploymentName,
			}
			// we delete the deployment either way
			defer func() {
				err := testclient.DataPlaneClient.Get(ctx, client.ObjectKey{Name: dp.Name, Namespace: testutils.NamespaceTesting}, dp)
				if err == nil {
					// deployment exists, delete it
					testlog.Infof("Deleting Deployment %v", dp.Name)
					err := testclient.Client.Delete(ctx, dp)
					Expect(err).ToNot(HaveOccurred())
				}
				testlog.Infof("Deployment %v is deleted %v", dp.Name)
			}()
			// we create and delete deployment in loop to create deployments in
			// quick succession to verify pre-start hook is able to write to
			// cpuset.cpus.exclusive
			for i := 0; i < 5; i++ {
				testlog.Infof("%d Create deployment %s with 2 Guaranteed pods requesting 2 cpus", i, DeploymentName)
				dp = deployments.Make(DeploymentName, testutils.NamespaceTesting,
					deployments.WithPodTemplate(p),
					deployments.WithNodeSelector(testutils.NodeSelectorLabels))
				dp.Spec.Template.Annotations = annotations
				dp.Spec.Template.Labels = deploymentLabels // Add labels to the pod template
				dp.Spec.Replicas = &numberofReplicas

				Expect(testclient.DataPlaneClient.Create(ctx, dp)).ToNot(HaveOccurred())
				podList := &corev1.PodList{}
				listOptions := &client.ListOptions{Namespace: testutils.NamespaceTesting, LabelSelector: labels.SelectorFromSet(deploymentLabels)}
				Eventually(func() bool {
					if err := testclient.DataPlaneClient.List(context.TODO(), podList, listOptions); err != nil {
						return false
					}
					for _, pod := range podList.Items {
						Expect(pod.Status.QOSClass).To(Equal(corev1.PodQOSGuaranteed))
						if isPodReady(&pod) || pod.Status.Phase == corev1.PodRunning {
							continue
						}
						for _, containerStatus := range pod.Status.ContainerStatuses {
							if containerStatus.State.Waiting == nil {
								continue
							}
							// we want to test container Pending state first so that we do not skip the condition
							// since we are creating and deleting the deployments, it's possible for this condition
							// to be skipped if one of the pods is not in pending state
							if containerStatus.State.Waiting.Reason == "RunContainerError" && strings.Contains(containerStatus.State.Waiting.Message, "failed to run pre-start hook for container") {
								testlog.Warningf("container %s failed to start with error: %s", pod.Spec.Containers[0].Name, containerStatus.State.Waiting.Message)
								return false
							}
						}
					}
					if len(podList.Items) < int(numberofReplicas) {
						testlog.Warningf("Required number of pods is %d", numberofReplicas)
						return false
					}
					for _, s := range podList.Items[0].Status.ContainerStatuses {
						if !s.Ready {
							testlog.Warningf("container status is %q", s.Name)
							return false
						}
					}
					// if we are here all the pods in the deployment are in ready state
					return true
				}, 10*time.Second, 5*time.Second).Should(BeTrue())
				// delete deployment
				// since the deployment is called in loop, all the pods are given some time
				// before we delete them because we want to preserve the current pod state
				// for some time to allow us to capture the container status messages before
				// we start deleting the pod
				testlog.Info("we wait for 5 seconds before deployment is deleted")
				time.Sleep(5 * time.Second)
				testlog.Infof("Deleting Deployment %v", dp.Name)
				err := testclient.Client.Delete(ctx, dp)
				Expect(err).ToNot(HaveOccurred())
			}
		})
	})
	Context("Crio Annotations", Label(string(label.Tier0)), func() {
		var testpod *corev1.Pod
		var allTestpods map[types.UID]*corev1.Pod
		var busyCpusImage string
		var targetNode = &corev1.Node{}
		annotations := map[string]string{
			"cpu-load-balancing.crio.io": "disable",
			"cpu-quota.crio.io":          "disable",
		}
		BeforeAll(func() {
			var err error
			allTestpods = make(map[types.UID]*corev1.Pod)
			busyCpusImage = busyCpuImageEnv()
			cpuRequest := 2
			testpod = getTestPodWithAnnotations(annotations, smtLevel)
			// workaround for https://github.com/kubernetes/kubernetes/issues/107074
			// until https://github.com/kubernetes/kubernetes/pull/120661 lands
			unblockerPod := pods.GetTestPod() // any non-GU pod will suffice ...
			unblockerPod.Namespace = testutils.NamespaceTesting
			unblockerPod.Spec.NodeSelector = map[string]string{testutils.LabelHostname: workerRTNode.Name}
			err = testclient.DataPlaneClient.Create(context.TODO(), unblockerPod)
			Expect(err).ToNot(HaveOccurred())
			allTestpods[unblockerPod.UID] = unblockerPod
			time.Sleep(30 * time.Second) // let cpumanager reconcile loop catch up

			// It's possible that when this test runs the value of
			// defaultCpuNotInSchedulingDomains is empty if no gu pods are running
			defaultCpuNotInSchedulingDomains, err := getCPUswithLoadBalanceDisabled(ctx, workerRTNode)
			Expect(err).ToNot(HaveOccurred(), "Unable to fetch scheduling domains")
			if len(defaultCpuNotInSchedulingDomains) > 0 {
				pods, err := pods.GetPodsOnNode(context.TODO(), workerRTNode.Name)
				if err != nil {
					testlog.Warningf("Warning cannot list pods on %q: %v", workerRTNode.Name, err)
				} else {
					testlog.Infof("pods on %q BEGIN", workerRTNode.Name)
					for _, pod := range pods {
						testlog.Infof("- %s/%s %s", pod.Namespace, pod.Name, pod.UID)
					}
					testlog.Infof("pods on %q END", workerRTNode.Name)
				}
				Expect(defaultCpuNotInSchedulingDomains).To(BeEmpty(), "the test expects all CPUs within a scheduling domain when starting")
			}

			By("Starting the pod")
			testpod.Spec.NodeSelector = testutils.NodeSelectorLabels
			runtimeClass := components.GetComponentName(profile.Name, components.ComponentNamePrefix)
			testpod.Spec.RuntimeClassName = &runtimeClass
			testpod.Spec.Containers[0].Image = busyCpusImage
			testpod.Spec.Containers[0].Resources.Limits[corev1.ResourceCPU] = resource.MustParse(fmt.Sprintf("%d", cpuRequest))

			if cpuRequest >= isolatedCPUSet.Size() {
				Skip(fmt.Sprintf("cpus request %d is greater than the available on the node as the isolated cpus are %d", cpuRequest, isolatedCPUSet.Size()))
			}

			err = testclient.DataPlaneClient.Create(context.TODO(), testpod)
			Expect(err).ToNot(HaveOccurred())
			testpod, err = pods.WaitForCondition(ctx, client.ObjectKeyFromObject(testpod), corev1.PodReady, corev1.ConditionTrue, 10*time.Minute)
			logEventsForPod(testpod)
			Expect(err).ToNot(HaveOccurred(), "failed to create guaranteed pod %v", testpod)
			allTestpods[testpod.UID] = testpod
			err = testclient.DataPlaneClient.Get(ctx, client.ObjectKey{Name: testpod.Spec.NodeName}, targetNode)
			Expect(err).ToNot(HaveOccurred(), "failed to fetch the node on which pod %v is running", testpod)
		})

		AfterAll(func() {
			for podUID, testpod := range allTestpods {
				testlog.Infof("deleting test pod %s/%s UID=%q", testpod.Namespace, testpod.Name, podUID)
				err := testclient.DataPlaneClient.Get(ctx, client.ObjectKeyFromObject(testpod), testpod)
				Expect(err).ToNot(HaveOccurred())
				err = testclient.DataPlaneClient.Delete(ctx, testpod)
				Expect(err).ToNot(HaveOccurred())

				err = pods.WaitForDeletion(ctx, testpod, pods.DefaultDeletionTimeout*time.Second)
				Expect(err).ToNot(HaveOccurred())
			}
		})

		Describe("cpuset controller", func() {
			It("[test_id:72080] Verify cpu affinity of container process matches with cpuset controller interface file cpuset.cpus", func() {
				cpusetCfg := &controller.CpuSet{}
				err := getter.Container(ctx, testpod, testpod.Spec.Containers[0].Name, cpusetCfg)
				Expect(err).ToNot(HaveOccurred())
				// Get cpus used by the container
				tasksetcmd := []string{"/bin/taskset", "-pc", "1"}
				testpodAffinity, err := pods.ExecCommandOnPod(testclient.K8sClient, testpod, testpod.Spec.Containers[0].Name, tasksetcmd)
				Expect(err).ToNot(HaveOccurred())
				podCpusStr := string(testpodAffinity)
				parts := strings.Split(strings.TrimSpace(podCpusStr), ":")
				testpodCpus := strings.TrimSpace(parts[1])
				testlog.Infof("%v pod is using %v cpus", testpod.Name, testpodCpus)
				podAffinityCpuset, err := cpuset.Parse(testpodCpus)
				Expect(err).ToNot(HaveOccurred(), "Unable to parse cpus %s used by %s pod", testpodCpus, testpod.Name)
				cgroupCpuset, err := cpuset.Parse(cpusetCfg.Cpus)
				Expect(err).ToNot(HaveOccurred(), "Unable to parse cpus from cgroups.cpuset")
				Expect(cgroupCpuset).To(Equal(podAffinityCpuset), "cpuset.cpus not matching the process affinity")
			})
		})
		Describe("Load Balancing Annotation", func() {
			It("[test_id:32646] cpus used by container should not be load balanced", func() {
				output, err := getPodCpus(testpod)
				Expect(err).ToNot(HaveOccurred(), "unable to fetch cpus used by testpod")
				podCpus, err := cpuset.Parse(output)
				Expect(err).ToNot(HaveOccurred(), "unable to parse cpuset used by pod")
				By("Getting the CPU scheduling flags")
				// After the testpod is started get the schedstat and check for cpus
				// not participating in scheduling domains
				checkSchedulingDomains(targetNode, podCpus, func(cpuIDs cpuset.CPUSet) error {
					if !podCpus.IsSubsetOf(cpuIDs) {
						return fmt.Errorf("pod CPUs NOT entirely part of cpus with load balance disabled: %v vs %v", podCpus, cpuIDs)
					}
					return nil
				}, 2*time.Minute, 5*time.Second, "checking scheduling domains with pod running")
			})

			It("[test_id:73382]cpuset.cpus.exclusive of kubepods.slice should be updated", func() {
				if !cgroupV2 {
					Skip("cpuset.cpus.exclusive is part of cgroupv2 interfaces")
				}
				cpusetCfg := &controller.CpuSet{}
				err := getter.Container(ctx, testpod, testpod.Spec.Containers[0].Name, cpusetCfg)
				Expect(err).ToNot(HaveOccurred())
				podCpuset, err := cpuset.Parse(cpusetCfg.Cpus)
				Expect(err).ToNot(HaveOccurred(), "Unable to parse pod cpus")
				kubepodsExclusiveCpus := fmt.Sprintf("%s/kubepods.slice/cpuset.cpus.exclusive", cgroupRoot)
				cmd := []string{"cat", kubepodsExclusiveCpus}
				out, err := nodes.ExecCommand(ctx, targetNode, cmd)
				Expect(err).ToNot(HaveOccurred())
				exclusiveCpus := testutils.ToString(out)
				exclusiveCpuset, err := cpuset.Parse(exclusiveCpus)
				Expect(err).ToNot(HaveOccurred(), "unable to parse cpuset.cpus.exclusive")
				Expect(podCpuset.Equals(exclusiveCpuset)).To(BeTrue())
			})
		})

		Describe("CPU Quota annotation", func() {
			It("[test_id:72079] CPU Quota interface files have correct values", func() {
				cpuCfg := &controller.Cpu{}
				err := getter.Container(ctx, testpod, testpod.Spec.Containers[0].Name, cpuCfg)
				Expect(err).ToNot(HaveOccurred())
				if cgroupV2 {
					Expect(cpuCfg.Quota).To(Equal("max"), "pod=%q, container=%q does not have quota set", client.ObjectKeyFromObject(testpod), testpod.Spec.Containers[0].Name)
				} else {
					Expect(cpuCfg.Quota).To(Equal("-1"), "pod=%q, container=%q does not have quota set", client.ObjectKeyFromObject(testpod), testpod.Spec.Containers[0].Name)
				}
			})

			It("[test_id: 72081] Verify cpu assigned to pod with quota disabled is not throttled", func() {
				cpuCfg := &controller.Cpu{}
				err := getter.Container(ctx, testpod, testpod.Spec.Containers[0].Name, cpuCfg)
				Expect(err).ToNot(HaveOccurred())
				Expect(cpuCfg.Stat["nr_throttled"]).To(Equal("0"), "cpu throttling not disabled on pod=%q, container=%q", client.ObjectKeyFromObject(testpod), testpod.Spec.Containers[0].Name)
			})
		})
	})

	Context("Check container runtimes cpu usage", Label(string(label.OpenShift)), func() {
		var guaranteedPod, bestEffortPod *corev1.Pod
		var guaranteedPodCpus, guaranteedInitPodCpus cpuset.CPUSet
		var bestEffortPodCpus, bestEffortInitPodCpus cpuset.CPUSet

		// What this test verifies:
		// - It checks the configuration flow from kubelet -> cri-o -> runc, ensuring the correct CPU
		//   configuration is provided to runc during the container creation process.
		//
		// What this test does NOT verify:
		// - It does not monitor the actual CPU usage during the container creation process or track
		//   the CPUs used by any intermediate forks.
		It("[test_id: 74461] Verify that runc excludes the cpus used by guaranteed pod", func() {
			By("Creating a guaranteed pod")
			guaranteedPod = makePod(ctx, workerRTNode, true)
			err := testclient.Client.Create(ctx, guaranteedPod)
			Expect(err).ToNot(HaveOccurred(), "Failed to create guaranteed pod")
			guaranteedPod, err = pods.WaitForCondition(ctx, client.ObjectKeyFromObject(guaranteedPod), corev1.PodReady, corev1.ConditionTrue, 10*time.Minute)
			Expect(err).ToNot(HaveOccurred())
			defer func() {
				if guaranteedPod != nil {
					testlog.Infof("deleting pod %q", guaranteedPod.Name)
					deleteTestPod(ctx, guaranteedPod)
				}
			}()

			// This Test is specific to runc container runtime
			// Skipping this test for crun.
			Eventually(func() error {
				expectedRuntime, err := runtime.GetContainerRuntimeTypeFor(context.TODO(), testclient.ControlPlaneClient, guaranteedPod)
				if err != nil {
					testlog.Errorf("Failed to fetch runtime for Guaranteed Pod: %v", err)
					return err
				}
				if expectedRuntime == "crun" {
					Skip(fmt.Sprintf("Skipping test as the runtime is 'crun', which is not the expected runtime. Found: %s", expectedRuntime))
				}
				return nil
			}).WithTimeout(30*time.Second).WithPolling(2*time.Second).Should(Succeed(), "Expected to successfully determine the container runtime type")

			By("Waiting for guaranteed pod to be ready")
			_, err = pods.WaitForCondition(ctx, client.ObjectKeyFromObject(guaranteedPod), corev1.PodReady, corev1.ConditionTrue, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Guaranteed pod did not become ready in time")
			Expect(guaranteedPod.Status.QOSClass).To(Equal(corev1.PodQOSGuaranteed), "Guaranteed pod does not have the correct QOSClass")
			testlog.Infof("Guaranteed pod %s/%s was successfully created", guaranteedPod.Namespace, guaranteedPod.Name)

			By("Creating a best-effort pod")
			bestEffortPod = makePod(ctx, workerRTNode, false)
			err = testclient.Client.Create(ctx, bestEffortPod)
			Expect(err).ToNot(HaveOccurred(), "Failed to create best-effort pod")
			defer func() {
				if bestEffortPod != nil {
					testlog.Infof("deleting pod %q", bestEffortPod.Name)
					deleteTestPod(ctx, bestEffortPod)
				}
			}()

			By("Waiting for best-effort pod to be ready")
			_, err = pods.WaitForCondition(ctx, client.ObjectKeyFromObject(bestEffortPod), corev1.PodReady, corev1.ConditionTrue, 5*time.Minute)
			Expect(err).ToNot(HaveOccurred(), "Best-effort pod did not become ready in time")
			testlog.Infof("BestEffort pod %s/%s was successfully created", bestEffortPod.Namespace, bestEffortPod.Name)

			By("Getting Information for guaranteed POD containers")
			GuPods := getConfigJsonInfo(guaranteedPod, "test", workerRTNode)
			for _, pod := range GuPods {
				switch pod.Annotations.ContainerName {
				case "test":
					guaranteedPodCpus, err = cpuset.Parse(pod.Linux.Resources.CPU.CPUs)
				case "POD":
					guaranteedInitPodCpus, err = cpuset.Parse(pod.Linux.Resources.CPU.CPUs)
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to parse GU POD cpus")
			}

			By("Getting Information for BestEffort POD containers")
			BEPods := getConfigJsonInfo(bestEffortPod, "test", workerRTNode)
			for _, pod := range BEPods {
				switch pod.Annotations.ContainerName {
				case "test":
					bestEffortPodCpus, err = cpuset.Parse(pod.Linux.Resources.CPU.CPUs)
				case "POD":
					bestEffortInitPodCpus, err = cpuset.Parse(pod.Linux.Resources.CPU.CPUs)
				}
				Expect(err).ToNot(HaveOccurred(), "Failed to parse BE POD cpus")
			}

			By("Validating CPU allocation for Guaranteed and Best-Effort pod containers")
			isolatedCpus, err := cpuset.Parse(string(*profile.Spec.CPU.Isolated))
			Expect(err).ToNot(HaveOccurred(), "Failed to parse isolated CPU set from performance profile")
			reservedCpus, err := cpuset.Parse(string(*profile.Spec.CPU.Reserved))
			Expect(err).ToNot(HaveOccurred(), "Failed to parse reserved CPU set from performance profile")

			Expect(guaranteedInitPodCpus.IsSubsetOf(reservedCpus)).
				To(BeTrue(), "Guaranteed Init pod CPUs (%s) are not strictly within the reserved set (%s)", guaranteedInitPodCpus, reservedCpus)
			Expect(guaranteedInitPodCpus.IsSubsetOf(isolatedCpus)).
				To(BeFalse(), "Guaranteed Init pod CPUs (%s) are within the isolated cpu set (%s)", guaranteedInitPodCpus, isolatedCpus)
			Expect(guaranteedPodCpus.IsSubsetOf(isolatedCpus)).
				To(BeTrue(), "Guaranteed pod CPUs (%s) are not strictly within the isolated set (%s)", guaranteedPodCpus, isolatedCpus)

			availableForBestEffort := isolatedCpus.Union(reservedCpus).Difference(guaranteedPodCpus)
			Expect(bestEffortInitPodCpus.IsSubsetOf(reservedCpus)).
				To(BeTrue(), "Best-Effort Init pod CPUs (%s) include CPUs not allowed (%s)", bestEffortInitPodCpus, availableForBestEffort)
			Expect(bestEffortPodCpus.IsSubsetOf(availableForBestEffort)).
				To(BeTrue(), "Best-Effort pod CPUs (%s) include CPUs not allowed (%s)", bestEffortPodCpus, availableForBestEffort)
		})
	})

})

func extractConfigInfo(output string) (*ContainerConfig, error) {
	var config ContainerConfig
	output = strings.TrimSpace(output)
	err := json.Unmarshal([]byte(output), &config)
	if err != nil {
		return nil, fmt.Errorf("failed to unmarshal config.json: %v", err)
	}
	return &config, nil
}

func getConfigJsonInfo(pod *corev1.Pod, containerName string, workerRTNode *corev1.Node) []*ContainerConfig {
	var pods []*ContainerConfig
	path := "/rootfs/var/lib/containers/storage/overlay-containers/"
	podName := pod.Name
	cmd := []string{
		"/bin/bash", "-c",
		fmt.Sprintf(
			`find %s -type f -exec grep -lP '\"io.kubernetes.pod.name\": \"%s\"' {} \; -exec grep -l '\"io.kubernetes.container.name\": \"%s\"' {} \; | sort -u`,
			path, podName, containerName,
		),
	}
	output, err := nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
	Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to search for config.json with podName %s and containerName %s", podName, containerName))
	filePaths := strings.Split(string(output), "\n")
	for _, filePath := range filePaths {
		if filePath == "" {
			continue
		}
		cmd = []string{"/bin/bash", "-c", fmt.Sprintf("cat %s", filePath)}
		output, err = nodes.ExecCommand(context.TODO(), workerRTNode, cmd)
		Expect(err).ToNot(HaveOccurred(), fmt.Sprintf("Failed to read config.json for container : %s", filePath))

		configData := testutils.ToString(output)
		config, err := extractConfigInfo(configData)
		if err != nil {
			testlog.Errorf("Error extracting config info:", err)
			continue
		}
		pods = append(pods, config)
		testlog.Infof("Pod Name: %s", config.Annotations.PodName)
		testlog.Infof("Container Name: %s", config.Annotations.ContainerName)
		testlog.Infof("Hostname: %s", config.Hostname)
		testlog.Infof("Arguments: %s", config.Process.Args)
		testlog.Infof("CPUs: %s", config.Linux.Resources.CPU.CPUs)
	}
	return pods
}

func makePod(ctx context.Context, workerRTNode *corev1.Node, guaranteed bool) *corev1.Pod {
	testPod := pods.GetTestPod()
	testPod.Namespace = testutils.NamespaceTesting
	testPod.Spec.NodeSelector = map[string]string{testutils.LabelHostname: workerRTNode.Name}
	if guaranteed {
		testPod.Spec.Containers[0].Resources = corev1.ResourceRequirements{
			Limits: corev1.ResourceList{
				corev1.ResourceCPU:    resource.MustParse("2"),
				corev1.ResourceMemory: resource.MustParse("200Mi"),
			},
		}
	}
	profile, _ := profiles.GetByNodeLabels(testutils.NodeSelectorLabels)
	runtimeClass := components.GetComponentName(profile.Name, components.ComponentNamePrefix)
	testPod.Spec.RuntimeClassName = &runtimeClass
	return testPod
}

func checkForWorkloadPartitioning(ctx context.Context) bool {
	// Look for the correct Workload Partition annotation in
	// a crio configuration file on the target node
	By("Check for Workload Partitioning enabled")
	cmd := []string{
		"chroot",
		"/rootfs",
		"/bin/bash",
		"-c",
		"echo CHECK ; /bin/grep -rEo 'activation_annotation.*target\\.workload\\.openshift\\.io/management.*' /etc/crio/crio.conf.d/ || true",
	}
	out, err := nodes.ExecCommand(ctx, workerRTNode, cmd)
	Expect(err).ToNot(HaveOccurred(), "Unable to check cluster for Workload Partitioning enabled")
	output := testutils.ToString(out)
	re := regexp.MustCompile(`activation_annotation.*target\.workload\.openshift\.io/management.*`)
	return re.MatchString(fmt.Sprint(output))
}

func checkPodHTSiblings(ctx context.Context, testpod *corev1.Pod) bool {
	By("Get test pod CPU list")
	containerID, err := pods.GetContainerIDByName(testpod, "test")
	Expect(err).ToNot(HaveOccurred(), "Unable to get pod containerID")

	cmd := []string{
		"chroot",
		"/rootfs",
		"/bin/bash",
		"-c",
		fmt.Sprintf("/bin/crictl inspect %s | /bin/jq -r '.info.runtimeSpec.linux.resources.cpu.cpus'", containerID),
	}
	node, err := nodes.GetByName(testpod.Spec.NodeName)
	Expect(err).ToNot(HaveOccurred(), "failed to get node %q", testpod.Spec.NodeName)
	Expect(testpod.Spec.NodeName).ToNot(BeEmpty(), "testpod %s/%s still pending - no nodeName set", testpod.Namespace, testpod.Name)
	out, err := nodes.ExecCommand(ctx, node, cmd)
	Expect(err).ToNot(HaveOccurred(), "Unable to crictl inspect containerID %q", containerID)
	output := testutils.ToString(out)
	podcpus, err := cpuset.Parse(strings.Trim(output, "\n"))
	Expect(err).ToNot(
		HaveOccurred(), "Unable to cpuset.Parse pod allocated cpu set from output %s", output)
	testlog.Infof("Test pod CPU list: %s", podcpus.String())

	// aggregate cpu sibling paris from the host based on the cpus allocated to the pod
	By("Get host cpu siblings for pod cpuset")
	hostHTSiblingPaths := strings.Builder{}
	for _, cpuNum := range podcpus.List() {
		_, err = hostHTSiblingPaths.WriteString(
			fmt.Sprintf(" /sys/devices/system/cpu/cpu%d/topology/thread_siblings_list", cpuNum),
		)
		Expect(err).ToNot(HaveOccurred(), "Build.Write failed to add dir path string?")
	}
	cmd = []string{
		"chroot",
		"/rootfs",
		"/bin/bash",
		"-c",
		fmt.Sprintf("/bin/cat %s | /bin/sort -u", hostHTSiblingPaths.String()),
	}
	out, err = nodes.ExecCommand(ctx, workerRTNode, cmd)
	Expect(err).ToNot(
		HaveOccurred(),
		"Unable to read host thread_siblings_list files",
	)
	output = testutils.ToString(out)

	// output is newline separated. Convert to cpulist format by replacing internal "\n" chars with ","
	hostHTSiblings := strings.ReplaceAll(
		strings.Trim(fmt.Sprint(output), "\n"), "\n", ",",
	)

	hostcpus, err := cpuset.Parse(hostHTSiblings)
	Expect(err).ToNot(HaveOccurred(), "Unable to parse host cpu HT siblings: %s", hostHTSiblings)
	By(fmt.Sprintf("Host CPU sibling set from querying for pod cpus: %s", hostcpus.String()))

	// pod cpu list should have the same siblings as the host for the same cpus
	return hostcpus.Equals(podcpus)
}

func startHTtestPod(ctx context.Context, cpuCount int) *corev1.Pod {
	var testpod *corev1.Pod

	annotations := map[string]string{}
	testpod = getTestPodWithAnnotations(annotations, cpuCount)
	testpod.Namespace = testutils.NamespaceTesting

	By(fmt.Sprintf("Creating test pod with %d cpus", cpuCount))
	testlog.Info(pods.DumpResourceRequirements(testpod))
	err := testclient.DataPlaneClient.Create(ctx, testpod)
	Expect(err).ToNot(HaveOccurred())
	testpod, err = pods.WaitForCondition(ctx, client.ObjectKeyFromObject(testpod), corev1.PodReady, corev1.ConditionTrue, 10*time.Minute)
	logEventsForPod(testpod)
	Expect(err).ToNot(HaveOccurred(), "Start pod failed")
	// Sanity check for QoS Class == Guaranteed
	Expect(testpod.Status.QOSClass).To(Equal(corev1.PodQOSGuaranteed),
		"Test pod does not have QoS class of Guaranteed")
	return testpod
}

func isSMTAlignmentError(pod *corev1.Pod) bool {
	re := regexp.MustCompile(`SMT.*Alignment.*Error`)
	return re.MatchString(pod.Status.Reason)
}

func getStressPod(nodeName string, cpus int) *corev1.Pod {
	cpuCount := fmt.Sprintf("%d", cpus)
	return &corev1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			GenerateName: "test-cpu-",
			Labels: map[string]string{
				"test": "",
			},
		},
		Spec: corev1.PodSpec{
			Containers: []corev1.Container{
				{
					Name:  "stress-test",
					Image: images.Test(),
					Resources: corev1.ResourceRequirements{
						Requests: corev1.ResourceList{
							corev1.ResourceCPU:    resource.MustParse(cpuCount),
							corev1.ResourceMemory: resource.MustParse("1Gi"),
						},
					},
					Command: []string{"/usr/bin/stresser"},
					Args:    []string{"-cpus", cpuCount},
				},
			},
			NodeSelector: map[string]string{
				testutils.LabelHostname: nodeName,
			},
		},
	}
}

func promotePodToGuaranteed(pod *corev1.Pod) *corev1.Pod {
	for idx := 0; idx < len(pod.Spec.Containers); idx++ {
		cnt := &pod.Spec.Containers[idx] // shortcut
		if cnt.Resources.Limits == nil {
			cnt.Resources.Limits = make(corev1.ResourceList)
		}
		for resName, resQty := range cnt.Resources.Requests {
			cnt.Resources.Limits[resName] = resQty
		}
	}
	return pod
}

func getTestPodWithProfileAndAnnotations(perfProf *performancev2.PerformanceProfile, annotations map[string]string, cpus int) *corev1.Pod {
	testpod := pods.GetTestPod()
	if len(annotations) > 0 {
		testpod.Annotations = annotations
	}
	testpod.Namespace = testutils.NamespaceTesting

	cpuCount := fmt.Sprintf("%d", cpus)

	resCpu := resource.MustParse(cpuCount)
	resMem := resource.MustParse("256Mi")

	// change pod resource requirements, to change the pod QoS class to guaranteed
	testpod.Spec.Containers[0].Resources = corev1.ResourceRequirements{
		Limits: corev1.ResourceList{
			corev1.ResourceCPU:    resCpu,
			corev1.ResourceMemory: resMem,
		},
	}

	if perfProf != nil {
		runtimeClassName := components.GetComponentName(perfProf.Name, components.ComponentNamePrefix)
		testpod.Spec.RuntimeClassName = &runtimeClassName
	}

	return testpod
}

func getTestPodWithAnnotations(annotations map[string]string, cpus int) *corev1.Pod {
	testpod := getTestPodWithProfileAndAnnotations(profile, annotations, cpus)

	testpod.Spec.NodeSelector = map[string]string{testutils.LabelHostname: workerRTNode.Name}

	return testpod
}

func deleteTestPod(ctx context.Context, testpod *corev1.Pod) (types.UID, bool) {
	// it possible that the pod already was deleted as part of the test, in this case we want to skip teardown
	err := testclient.DataPlaneClient.Get(ctx, client.ObjectKeyFromObject(testpod), testpod)
	if errors.IsNotFound(err) {
		return "", false
	}

	testpodUID := testpod.UID

	err = testclient.DataPlaneClient.Delete(ctx, testpod)
	Expect(err).ToNot(HaveOccurred())

	err = pods.WaitForDeletion(ctx, testpod, pods.DefaultDeletionTimeout*time.Second)
	Expect(err).ToNot(HaveOccurred())

	return testpodUID, true
}

func cpuSpecToString(cpus *performancev2.CPU) (string, error) {
	if cpus == nil {
		return "", fmt.Errorf("performance CPU field is nil")
	}
	sb := strings.Builder{}
	if cpus.Reserved != nil {
		_, err := fmt.Fprintf(&sb, "reserved=[%s]", *cpus.Reserved)
		if err != nil {
			return "", err
		}
	}
	if cpus.Isolated != nil {
		_, err := fmt.Fprintf(&sb, " isolated=[%s]", *cpus.Isolated)
		if err != nil {
			return "", err
		}
	}
	if cpus.BalanceIsolated != nil {
		_, err := fmt.Fprintf(&sb, " balanceIsolated=%t", *cpus.BalanceIsolated)
		if err != nil {
			return "", err
		}
	}
	return sb.String(), nil
}

func logEventsForPod(testPod *corev1.Pod) {
	evs, err := events.GetEventsForObject(testclient.DataPlaneClient, testPod.Namespace, testPod.Name, string(testPod.UID))
	if err != nil {
		testlog.Error(err)
	}
	for _, event := range evs.Items {
		testlog.Warningf("-> %s %s %s", event.Action, event.Reason, event.Message)
	}
}

// getCPUswithLoadBalanceDisabled Return cpus which are not in any scheduling domain
func getCPUswithLoadBalanceDisabled(ctx context.Context, targetNode *corev1.Node) ([]string, error) {
	cmd := []string{"/bin/bash", "-c", "cat /proc/schedstat"}
	out, err := nodes.ExecCommand(ctx, targetNode, cmd)
	if err != nil {
		return nil, err
	}
	schedstatData := testutils.ToString(out)

	info, err := schedstat.ParseData(strings.NewReader(schedstatData))
	if err != nil {
		return nil, err
	}

	cpusWithoutDomain := []string{}
	for _, cpu := range info.GetCPUs() {
		doms, ok := info.GetDomains(cpu)
		if !ok {
			return nil, fmt.Errorf("unknown cpu: %v", cpu)
		}
		if len(doms) > 0 {
			continue
		}
		cpusWithoutDomain = append(cpusWithoutDomain, cpu)
	}

	return cpusWithoutDomain, nil
}

// getPodCpus return cpus used based on taskset
func getPodCpus(testpod *corev1.Pod) (string, error) {
	tasksetcmd := []string{"taskset", "-pc", "1"}
	testpodCpusByte, err := pods.ExecCommandOnPod(testclient.K8sClient, testpod, testpod.Spec.Containers[0].Name, tasksetcmd)
	if err != nil {
		return "", err
	}
	testpodCpusStr := string(testpodCpusByte)
	parts := strings.Split(strings.TrimSpace(testpodCpusStr), ":")
	cpus := strings.TrimSpace(parts[1])
	return cpus, err
}

// checkSchedulingDomains Check cpus are part of any scheduling domain
func checkSchedulingDomains(workerRTNode *corev1.Node, podCpus cpuset.CPUSet, testFunc func(cpuset.CPUSet) error, timeout, polling time.Duration, errMsg string) {
	Eventually(func() error {
		cpusNotInSchedulingDomains, err := getCPUswithLoadBalanceDisabled(context.TODO(), workerRTNode)
		Expect(err).ToNot(HaveOccurred())
		testlog.Infof("cpus with load balancing disabled are: %v", cpusNotInSchedulingDomains)
		Expect(err).ToNot(HaveOccurred(), "unable to fetch cpus with load balancing disabled from /proc/schedstat")
		cpuIDList, err := schedstat.MakeCPUIDListFromCPUList(cpusNotInSchedulingDomains)
		if err != nil {
			return err
		}
		cpuIDs := cpuset.New(cpuIDList...)
		return testFunc(cpuIDs)
	}).WithTimeout(2*time.Minute).WithPolling(5*time.Second).ShouldNot(HaveOccurred(), errMsg)
}

// busyCpuImageEnv return busycpus image used for crio quota annotations test
// This is required for running tests on disconnected environment where images are mirrored
// in private registries.
func busyCpuImageEnv() string {
	qeImageRegistry, ok := os.LookupEnv("IMAGE_REGISTRY")
	if !ok {
		qeImageRegistry = "quay.io/ocp-edge-qe/"
	}

	busyCpusImage, ok := os.LookupEnv("BUSY_CPUS_IMAGE")
	if !ok {
		busyCpusImage = "busycpus"
	}

	return fmt.Sprintf("%s%s", qeImageRegistry, busyCpusImage)
}

// isPodReady checks if the pod is in ready state
func isPodReady(pod *corev1.Pod) bool {
	for _, condition := range pod.Status.Conditions {
		if condition.Type == corev1.PodReady && condition.Status == corev1.ConditionTrue {
			return true
		}
	}
	return false
}
