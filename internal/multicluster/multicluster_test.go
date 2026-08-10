package multicluster

import (
	"context"
	"testing"
	"time"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/runtime"
	"k8s.io/apimachinery/pkg/types"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/rest"
	ctrl "sigs.k8s.io/controller-runtime"
	"sigs.k8s.io/controller-runtime/pkg/client"
	"sigs.k8s.io/controller-runtime/pkg/client/fake"

	clusterv1 "sigs.k8s.io/cluster-api/api/core/v1beta2"

	"github.com/rpcu/dike/internal/controller"
)

func TestMulticlusterManager_Reconcile(t *testing.T) {
	scheme := runtime.NewScheme()
	_ = corev1.AddToScheme(scheme)
	_ = clusterv1.AddToScheme(scheme)

	ns := "mgmt"
	clusterName := "test-cluster"
	req := ctrl.Request{
		NamespacedName: types.NamespacedName{
			Namespace: ns,
			Name:      clusterName,
		},
	}

	t.Run("cluster not found -> no error", func(t *testing.T) {
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).Build()
		mgr := &Manager{
			Client:   fakeClient,
			LabelKey: DefaultLabelSelector,
		}

		res, err := mgr.Reconcile(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Requeue || res.RequeueAfter > 0 {
			t.Errorf("expected no requeue, got %v", res)
		}
	})

	t.Run("cluster without label -> watcher stopped if running", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: ns,
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
		mgr := &Manager{
			Client:   fakeClient,
			LabelKey: DefaultLabelSelector,
		}

		res, err := mgr.Reconcile(context.Background(), req)
		if err != nil {
			t.Fatalf("unexpected error: %v", err)
		}
		if res.Requeue {
			t.Errorf("expected no requeue, got %v", res)
		}
	})

	t.Run("cluster labeled true but missing secret -> error", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: ns,
				Labels: map[string]string{
					DefaultLabelSelector: "true",
				},
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster).Build()
		mgr := &Manager{
			Client:   fakeClient,
			LabelKey: DefaultLabelSelector,
		}

		_, err := mgr.Reconcile(context.Background(), req)
		if err == nil {
			t.Fatal("expected error for missing secret, got nil")
		}
	})

	t.Run("cluster labeled true with invalid secret -> error", func(t *testing.T) {
		cluster := &clusterv1.Cluster{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName,
				Namespace: ns,
				Labels: map[string]string{
					DefaultLabelSelector: "true",
				},
			},
		}
		secret := &corev1.Secret{
			ObjectMeta: metav1.ObjectMeta{
				Name:      clusterName + "-kubeconfig",
				Namespace: ns,
			},
			Data: map[string][]byte{
				"value": []byte("invalid-kubeconfig-yaml"),
			},
		}
		fakeClient := fake.NewClientBuilder().WithScheme(scheme).WithObjects(cluster, secret).Build()
		mgr := &Manager{
			Client:   fakeClient,
			LabelKey: DefaultLabelSelector,
		}

		_, err := mgr.Reconcile(context.Background(), req)
		if err == nil {
			t.Fatal("expected error for invalid kubeconfig, got nil")
		}
	})

	t.Run("watcher stop method", func(t *testing.T) {
		stopped := false
		w := &Watcher{
			ClusterName: "mgmt/test",
			cancel: func() {
				stopped = true
			},
		}
		w.Stop()
		if !stopped {
			t.Error("expected watcher cancel function to be invoked")
		}
	})
}

func TestWatcher_Start_Dummy(t *testing.T) {
	ctx, cancel := context.WithTimeout(context.Background(), 100*time.Millisecond)
	defer cancel()

	w := &Watcher{
		ClusterName: "test/dummy",
		RESTConfig:  &rest.Config{Host: "https://127.0.0.1:6443"},
		LRPReconcilerFn: func(c client.Client, cs kubernetes.Interface, rc *rest.Config) *controller.LRPReconciler {
			return &controller.LRPReconciler{}
		},
	}
	// Start with mock/cancelled context should return or terminate cleanly
	_ = w.Start(ctx)
}
