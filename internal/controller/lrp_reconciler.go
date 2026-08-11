// Package controller implements the Kubernetes controller that watches
// CiliumLocalRedirectPolicy resources and ensures the Cilium eBPF
// datapath has correctly applied the local redirect.
package controller

import (
	"context"
	"fmt"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rpcu/dike/internal/cilium"
)

var CiliumLRPGVK = schema.GroupVersionKind{
	Group:   "cilium.io",
	Version: "v2",
	Kind:    "CiliumLocalRedirectPolicy",
}

const requeueDelay = 60 * time.Second

type LRPReconciler struct {
	client.Client

	Clientset kubernetes.Interface

	RESTConfig *rest.Config

	// ClusterName identifies the tenant cluster this reconciler is bound to.
	// Used to generate a unique controller name per tenant.
	ClusterName string
}

// NewLRPReconciler creates a new LRPReconciler bound to the given tenant
// cluster client and rest config.
func NewLRPReconciler(c client.Client, clientset kubernetes.Interface, restConfig *rest.Config, clusterName string) *LRPReconciler {
	return &LRPReconciler{
		Client:      c,
		Clientset:   clientset,
		RESTConfig:  restConfig,
		ClusterName: clusterName,
	}
}

func (r *LRPReconciler) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)

	// (1) Vérifier que la LRP existe encore.
	lrp := &unstructured.Unstructured{}
	lrp.SetGroupVersionKind(CiliumLRPGVK)
	if err := r.Get(ctx, req.NamespacedName, lrp); err != nil {
		if apierrors.IsNotFound(err) {
			log.Info("LRP deleted, nothing to do")
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	svc := &corev1.Service{}
	key := types.NamespacedName{Name: "kubernetes", Namespace: "default"}
	if err := r.Get(ctx, key, svc); err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to get kubernetes service: %w", err)
	}
	clusterIP := svc.Spec.ClusterIP
	log.Info("target ClusterIP resolved", "ip", clusterIP)

	checker := cilium.NewChecker(r.Clientset, r.RESTConfig)
	results, err := checker.CheckNodes(ctx, clusterIP)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("failed to check nodes: %w", err)
	}

	restarted := 0
	for _, result := range results {
		if result.Err != nil {
			log.Info("error checking node via exec, attempting cilium agent restart to unblock node", "node", result.NodeName, "pod", result.PodName, "error", result.Err)
		} else if result.IsRedirect {
			log.Info("already applied", "node", result.NodeName)
			continue
		}
		pod := &corev1.Pod{}
		pod.Name = result.PodName
		pod.Namespace = "kube-system"
		if err := r.Delete(ctx, pod); err != nil {
			log.Error(err, "failed to delete cilium pod", "pod", result.PodName)
			continue
		}
		log.Info("restarted cilium agent", "node", result.NodeName, "pod", result.PodName)
		restarted++
	}

	if restarted > 0 {
		log.Info("restarted cilium agents, will recheck", "count", restarted)
		return ctrl.Result{RequeueAfter: requeueDelay}, nil
	}

	return ctrl.Result{}, nil
}

func (r *LRPReconciler) SetupWithManager(mgr ctrl.Manager) error {
	lrp := &unstructured.Unstructured{}
	lrp.SetGroupVersionKind(CiliumLRPGVK)

	name := "lrp-reconciler"
	if r.ClusterName != "" {
		name = fmt.Sprintf("lrp-reconciler-%s", r.ClusterName)
	}

	return ctrl.NewControllerManagedBy(mgr).
		For(lrp).
		Named(name).
		Complete(r)
}
