package leaderelection

import (
	"context"
	"fmt"
	"os"

	"github.com/samber/lo"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/util/uuid"
	coordinationv1client "k8s.io/client-go/kubernetes/typed/coordination/v1"
	corev1client "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/leaderelection/resourcelock"
	"sigs.k8s.io/controller-runtime/pkg/log"
)

// LeaseHijacker implements lease stealing to accelerate development workflows
// TODO: migrate to https://kubernetes.io/docs/concepts/cluster-administration/coordinated-leader-election/ when it's past alpha.
func LeaseHijacker(ctx context.Context, config *rest.Config, namespace string, name string) resourcelock.Interface {
	if os.Getenv("HIJACK_LEASE") != "true" {
		return nil // If not set, fallback to other controller-runtime lease settings
	}
	id := fmt.Sprintf("%s_%s", lo.Must(os.Hostname()), uuid.NewUUID())
	log.FromContext(ctx).Info("hijacking lease", "namespace", namespace, "name", name)
	kubeClient := coordinationv1client.NewForConfigOrDie(config)
	lease := lo.Must(kubeClient.Leases(namespace).Get(ctx, name, metav1.GetOptions{}))
	lease.Spec.HolderIdentity = lo.ToPtr(id)
	lease.Spec.AcquireTime = lo.ToPtr(metav1.NowMicro())
	lo.Must(kubeClient.Leases(namespace).Update(ctx, lease, metav1.UpdateOptions{}))
	return lo.Must(resourcelock.New(
		resourcelock.LeasesResourceLock,
		namespace,
		name,
		corev1client.NewForConfigOrDie(config),
		coordinationv1client.NewForConfigOrDie(config),
		resourcelock.ResourceLockConfig{Identity: id},
	))
}
