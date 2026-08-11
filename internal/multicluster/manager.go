// Package multicluster provides a manager that discovers tenant clusters
// via CAPI Cluster objects on the management cluster and runs a
// CiliumLocalRedirectPolicy reconciler for each of them.
package multicluster

import (
	"context"
	"fmt"
	"sync"
	"time"

	corev1 "k8s.io/api/core/v1"
	apierrors "k8s.io/apimachinery/pkg/api/errors"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/tools/clientcmd"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/log"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	"github.com/rpcu/dike/internal/controller"
)

const (
	// DefaultLabelSelector is the label that must be set on Cluster objects
	// for dike to manage them.
	DefaultLabelSelector = "dike.rpcu.io/enabled"

	// kubeconfigSecretSuffix is appended to the Cluster name to derive
	// the Secret that contains the admin kubeconfig.
	kubeconfigSecretSuffix = "-kubeconfig"

	// kubeconfigSecretKey is the data key inside the CAPI kubeconfig Secret.
	kubeconfigSecretKey = "value"
)

// Manager reconciles CAPI Cluster objects and starts / stops per-tenant
// watchers accordingly.
type Manager struct {
	client.Client

	LabelKey string

	// watchers keeps track of running per-tenant watchers keyed by
	// "<namespace>/<name>".
	watchers sync.Map // map[string]*Watcher
}

// +kubebuilder:rbac:groups=cluster.x-k8s.io,resources=clusters,verbs=get;list;watch
// +kubebuilder:rbac:groups="",resources=secrets,verbs=get;list;watch

func (m *Manager) Reconcile(ctx context.Context, req ctrl.Request) (ctrl.Result, error) {
	log := log.FromContext(ctx)
	key := req.NamespacedName.String()

	// Fetch the Cluster object.
	cluster := &clusterv1.Cluster{}
	if err := m.Get(ctx, req.NamespacedName, cluster); err != nil {
		if apierrors.IsNotFound(err) {
			// Cluster was deleted — stop the watcher if running.
			m.stopWatcher(key, log)
			return ctrl.Result{}, nil
		}
		return ctrl.Result{}, err
	}

	// Check if the Cluster has the required label.
	labelKey := m.LabelKey
	if labelKey == "" {
		labelKey = DefaultLabelSelector
	}
	if cluster.Labels[labelKey] != "true" {
		m.stopWatcher(key, log)
		return ctrl.Result{}, nil
	}

	if _, loaded := m.watchers.Load(key); loaded {
		log.V(1).Info("Watcher already running", "cluster", key)
		return ctrl.Result{}, nil
	}

	secretName := cluster.Name + kubeconfigSecretSuffix
	secret := &corev1.Secret{}
	secretKey := types.NamespacedName{Name: secretName, Namespace: cluster.Namespace}
	if err := m.Get(ctx, secretKey, secret); err != nil {
		return ctrl.Result{}, fmt.Errorf("reading kubeconfig Secret %s: %w", secretKey, err)
	}

	kubeconfigBytes, ok := secret.Data[kubeconfigSecretKey]
	if !ok {
		return ctrl.Result{}, fmt.Errorf("Secret %s has no key %q", secretKey, kubeconfigSecretKey)
	}

	restConfig, err := clientcmd.RESTConfigFromKubeConfig(kubeconfigBytes)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("parsing kubeconfig for cluster %s: %w", key, err)
	}

	clientset, err := kubernetes.NewForConfig(restConfig)
	if err != nil {
		return ctrl.Result{}, fmt.Errorf("creating clientset for cluster %s: %w", key, err)
	}

	watcher := &Watcher{
		ClusterName:     key,
		RESTConfig:      restConfig,
		Clientset:       clientset,
		LRPReconcilerFn: controller.NewLRPReconciler,
	}

	watcherCtx, cancel := context.WithCancel(ctx)
	watcher.cancel = cancel
	m.watchers.Store(key, watcher)

	log.Info("Starting watcher for tenant cluster", "cluster", key)
	go func() {
		if err := watcher.Start(watcherCtx); err != nil {
			log.Error(err, "Watcher stopped with error, will retry", "cluster", key)
		}
		m.watchers.Delete(key)
	}()

	return ctrl.Result{RequeueAfter: 30 * time.Second}, nil
}

func (m *Manager) stopWatcher(key string, log interface{ Info(string, ...any) }) {
	if val, loaded := m.watchers.LoadAndDelete(key); loaded {
		w := val.(*Watcher)
		log.Info("Stopping watcher for tenant cluster", "cluster", key)
		w.Stop()
	}
}

func (m *Manager) SetupWithManager(mgr ctrl.Manager) error {
	return ctrl.NewControllerManagedBy(mgr).
		For(&clusterv1.Cluster{}).
		Named("multicluster-manager").
		Complete(m)
}
