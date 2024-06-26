package node_inspector

import (
	"context"
	"fmt"

	appsv1 "k8s.io/api/apps/v1"
	corev1 "k8s.io/api/core/v1"
	rbacv1 "k8s.io/api/rbac/v1"
	"k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/api/resource"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/klog"
	"k8s.io/utils/pointer"
	"sigs.k8s.io/controller-runtime/pkg/client"

	testutils "github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils"
	testclient "github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/client"
	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/daemonset"
	testlog "github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/log"
	testpods "github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/pods"
)

const serviceAccountSuffix = "sa"
const clusterRoleSuffix = "cr"
const clusterRoleBindingSuffix = "crb"

func Create(cli client.Client, namespace, name, image string) error {
	serviceAccountName := fmt.Sprintf("%s-%s", name, serviceAccountSuffix)
	sa := createServiceAccount(serviceAccountName, namespace)
	if err := cli.Create(context.Background(), sa); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	clusterRoleName := fmt.Sprintf("%s-%s", name, clusterRoleSuffix)
	cr := createClusterRole(clusterRoleName)
	if err := cli.Create(context.Background(), cr); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	clusterRoleBindingName := fmt.Sprintf("%s-%s", name, clusterRoleBindingSuffix)
	rb := createClusterRoleBinding(clusterRoleBindingName, namespace, serviceAccountName, clusterRoleName)
	if err := cli.Create(context.Background(), rb); err != nil && !errors.IsAlreadyExists(err) {
		return err
	}
	ds := createDaemonSet(name, namespace, serviceAccountName, image)
	if err := cli.Create(context.Background(), ds); err != nil {
		if !errors.IsAlreadyExists(err) {
			return err
		}
		klog.Infof("The node inspector daemonset was not expected to be running")
	}
	if err := daemonset.WaitToBeRunning(cli, namespace, name); err != nil {
		return err
	}

	return nil
}

func Delete(cli client.Client, namespace, name string) error {
	ns := &corev1.Namespace{ObjectMeta: metav1.ObjectMeta{Name: namespace}}
	if err := cli.Delete(context.Background(), ns); err != nil && !errors.IsNotFound(err) {
		return err
	}
	cr := &rbacv1.ClusterRole{ObjectMeta: metav1.ObjectMeta{Name: fmt.Sprintf("%s-%s", name, clusterRoleSuffix)}}
	if err := cli.Delete(context.Background(), cr); err != nil && !errors.IsNotFound(err) {
		return err
	}
	return nil
}

func isRunning(cli client.Client, namespace, name string) (bool, error) {
	return daemonset.IsRunning(cli, namespace, name)
}

// getDaemonPodByNode returns the daemon pod that runs on the specified node
func getDaemonPodByNode(node *corev1.Node) (*corev1.Pod, error) {
	listOptions := &client.ListOptions{
		Namespace:     testutils.NodeInspectorNamespace,
		FieldSelector: fields.SelectorFromSet(fields.Set{"spec.nodeName": node.Name}),
		LabelSelector: labels.SelectorFromSet(labels.Set{"name": testutils.NodeInspectorName}),
	}

	pods := &corev1.PodList{}
	if err := testclient.DataPlaneClient.List(context.TODO(), pods, listOptions); err != nil {
		return nil, err
	}
	if len(pods.Items) < 1 {
		return nil, fmt.Errorf("failed to get daemon pod for the node %q", node.Name)
	}
	return &pods.Items[0], nil
}

// ExecCommand executing the command on a daemon pod of the given node
func ExecCommand(ctx context.Context, node *corev1.Node, command []string) ([]byte, error) {
	// Ensure the node inspector is running
	ok, err := isRunning(testclient.DataPlaneClient, testutils.NodeInspectorNamespace, testutils.NodeInspectorName)
	if err != nil || !ok {
		return nil, err
	}
	pod, err := getDaemonPodByNode(node)
	if err != nil {
		return nil, err
	}
	testlog.Infof("found daemon pod %s for node %s", pod.Name, node.Name)

	return testpods.WaitForPodOutput(ctx, testclient.K8sClient, pod, command)
}

func createDaemonSet(name, namespace, serviceAccountName, image string) *appsv1.DaemonSet {
	MountPropagationHostToContainer := corev1.MountPropagationHostToContainer
	return &appsv1.DaemonSet{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
			Labels: map[string]string{
				"name": name,
			},
		},
		Spec: appsv1.DaemonSetSpec{
			Selector: &metav1.LabelSelector{
				MatchLabels: map[string]string{
					"name": name,
				},
			},
			Template: corev1.PodTemplateSpec{
				ObjectMeta: metav1.ObjectMeta{
					Annotations: map[string]string{
						"target.workload.openshift.io/management": `{"effect": "PreferredDuringScheduling"}`,
					},
					Labels: map[string]string{
						"name": name,
					},
				},
				Spec: corev1.PodSpec{
					HostPID:                       true,
					HostNetwork:                   true,
					ServiceAccountName:            serviceAccountName,
					TerminationGracePeriodSeconds: pointer.Int64(0),
					NodeSelector:                  map[string]string{"kubernetes.io/os": "linux"},
					Containers: []corev1.Container{
						{
							Name:            "node-daemon",
							Image:           image,
							Command:         []string{"/bin/bash", "-c", "sleep INF"},
							ImagePullPolicy: corev1.PullAlways,
							Resources: corev1.ResourceRequirements{
								Requests: corev1.ResourceList{
									"cpu":    resource.MustParse("20m"),
									"memory": resource.MustParse("50Mi"),
								},
							},
							SecurityContext: &corev1.SecurityContext{
								Privileged:             pointer.Bool(true),
								ReadOnlyRootFilesystem: pointer.Bool(true),
							},
							VolumeMounts: []corev1.VolumeMount{
								{
									MountPath:        "/rootfs",
									Name:             "rootfs",
									MountPropagation: &MountPropagationHostToContainer,
								},
							},
						},
					},
					Volumes: []corev1.Volume{
						{
							Name: "rootfs",
							VolumeSource: corev1.VolumeSource{
								HostPath: &corev1.HostPathVolumeSource{
									Path: "/",
								},
							},
						},
					},
					Tolerations: []corev1.Toleration{
						{
							Operator: corev1.TolerationOpExists,
						},
					},
				},
			},
		},
	}
}

func createServiceAccount(name, namespace string) *corev1.ServiceAccount {
	return &corev1.ServiceAccount{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
	}
}

func createClusterRole(name string) *rbacv1.ClusterRole {
	return &rbacv1.ClusterRole{
		ObjectMeta: metav1.ObjectMeta{
			Name: name,
		},
		Rules: []rbacv1.PolicyRule{
			{
				APIGroups:     []string{"security.openshift.io"},
				ResourceNames: []string{"privileged"},
				Resources:     []string{"securitycontextconstraints"},
				Verbs:         []string{"use"},
			},
		},
	}
}

func createClusterRoleBinding(name, namespace, serviceAccountName, clusterRoleName string) *rbacv1.RoleBinding {
	return &rbacv1.RoleBinding{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: namespace,
		},
		Subjects: []rbacv1.Subject{
			{
				Kind:      "ServiceAccount",
				Name:      serviceAccountName,
				Namespace: namespace,
			},
		},
		RoleRef: rbacv1.RoleRef{
			Kind:     "ClusterRole",
			Name:     clusterRoleName,
			APIGroup: "rbac.authorization.k8s.io",
		},
	}
}
