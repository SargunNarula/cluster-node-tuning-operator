package infrastructure

import (
	"context"
	"strings"

	corev1 "k8s.io/api/core/v1"

	"github.com/openshift/cluster-node-tuning-operator/test/e2e/performanceprofile/functests/utils/nodes"
)

// IsVM checks if a given node's underlying infrastructure is a VM
func IsVM(ctx context.Context, node *corev1.Node) (bool, error) {
	cmd := []string{
		"/usr/sbin/chroot",
		"/rootfs",
		"/bin/bash", "-c",
		"systemd-detect-virt > /dev/null; echo $?",
	}
	output, err := nodes.ExecCommand(ctx, node, cmd)
	if err != nil {
		return false, err
	}

	statusCode := strings.TrimSpace(string(output))
	isVM := statusCode == "0"

	return isVM, nil
}
