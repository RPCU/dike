package multicluster

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	clientgoscheme "k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	"github.com/rpcu/dike/internal/controller"
)

// LRPReconcilerFactory creates an LRPReconciler for a given tenant cluster.
type LRPReconcilerFactory func(c client.Client, clientset kubernetes.Interface, restConfig *rest.Config, clusterName string) *controller.LRPReconciler

type Watcher struct {
	// ClusterName is the "<namespace>/<name>" of the CAPI Cluster.
	ClusterName string

	// RESTConfig is the rest.Config built from the tenant kubeconfig.
	RESTConfig *rest.Config

	// Clientset is a kubernetes.Interface for the tenant cluster.
	Clientset kubernetes.Interface

	LRPReconcilerFn LRPReconcilerFactory

	cancel context.CancelFunc
}

func (w *Watcher) Start(ctx context.Context) error {
	log := log.FromContext(ctx).WithValues("tenant", w.ClusterName)
	log.Info("Initializing tenant watcher")

	scheme := runtime.NewScheme()
	if err := clientgoscheme.AddToScheme(scheme); err != nil {
		return fmt.Errorf("adding client-go scheme: %w", err)
	}

	tenantMgr, err := ctrl.NewManager(w.RESTConfig, ctrl.Options{
		Scheme: scheme,

		Metrics:                ctrl.Options{}.Metrics,
		HealthProbeBindAddress: "",

		LeaderElection: false,
	})
	if err != nil {
		return fmt.Errorf("creating tenant manager for %s: %w", w.ClusterName, err)
	}

	factory := w.LRPReconcilerFn
	if factory == nil {
		factory = controller.NewLRPReconciler
	}

	reconciler := factory(tenantMgr.GetClient(), w.Clientset, w.RESTConfig, w.ClusterName)
	if err := reconciler.SetupWithManager(tenantMgr); err != nil {
		return fmt.Errorf("setting up LRP reconciler for %s: %w", w.ClusterName, err)
	}

	log.Info("Starting tenant manager")
	return tenantMgr.Start(ctx)
}

func (w *Watcher) Stop() {
	if w.cancel != nil {
		w.cancel()
	}
}
