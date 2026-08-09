// Package cilium provides helpers to inspect the Cilium eBPF datapath.
//
// The main use-case is checking whether a given ClusterIP is handled
// as a LocalRedirect service (meaning the CiliumLocalRedirectPolicy
// has been correctly applied by the Cilium agent on that node).
package cilium

import (
	"bytes"
	"context"
	"encoding/json"
	"fmt"

	corev1 "k8s.io/api/core/v1"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

// ---------------------------------------------------------------
// Constantes
// ---------------------------------------------------------------

const (
	// CiliumNamespace is where the Cilium DaemonSet runs.
	CiliumNamespace = "kube-system"

	// CiliumAgentLabel is the label selector for Cilium agent pods.
	// Vérifie sur ton cluster avec : kubectl get pods -n kube-system -l app.kubernetes.io/name=cilium-agent
	CiliumAgentLabel = "app.kubernetes.io/name=cilium-agent"
)

type CheckResult struct {
	NodeName   string
	PodName    string
	IsRedirect bool
	Err        error
}

type ciliumFrontend struct {
	IP       string `json:"ip"`
	Port     int    `json:"port"`
	Protocol string `json:"protocol"`
}
type ciliumServiceSpec struct {
	FrontendAddress ciliumFrontend    `json:"frontend-address"`
	Flags           map[string]string `json:"flags"`
}

type ciliumService struct {
	Spec ciliumServiceSpec `json:"spec"`
}

type Checker struct {
	clientset  kubernetes.Interface
	restConfig *rest.Config
}

func NewChecker(clientset kubernetes.Interface, restConfig *rest.Config) *Checker {
	return &Checker{
		clientset:  clientset,
		restConfig: restConfig,
	}
}

func (c *Checker) CheckNodes(ctx context.Context, clusterIP string) ([]CheckResult, error) {
	podList, err := c.clientset.CoreV1().Pods(CiliumNamespace).List(ctx, metav1.ListOptions{LabelSelector: CiliumAgentLabel})
	if err != nil {
		return nil, fmt.Errorf("impossible de lister les pods cilium: %w", err)
	}

	var results []CheckResult
	for _, pod := range podList.Items {
		nodeName := pod.Spec.NodeName
		output, err := c.execInPod(ctx, CiliumNamespace, pod.Name, []string{"cilium-dbg", "service", "list", "-o", "json"})
		if err != nil {
			results = append(results, CheckResult{NodeName: nodeName, PodName: pod.Name, Err: err})
			continue
		}

		analyseOutput, err := c.parseServiceList(output, clusterIP)
		if err != nil {
			results = append(results, CheckResult{NodeName: nodeName, PodName: pod.Name, Err: err})
			continue
		}
		results = append(results, CheckResult{
			NodeName:   nodeName,
			PodName:    pod.Name,
			IsRedirect: analyseOutput,
		})
	}
	return results, nil
}

func (c *Checker) execInPod(ctx context.Context, namespace, podName string, command []string) (string, error) {
	req := c.clientset.CoreV1().RESTClient().Post().Resource("pods").Name(podName).Namespace(namespace).SubResource("exec")

	req.VersionedParams(&corev1.PodExecOptions{Command: command, Stdout: true, Stderr: true}, scheme.ParameterCodec)

	execURL := req.URL()

	wsExecutor, wsErr := remotecommand.NewWebSocketExecutor(c.restConfig, "POST", execURL.String())
	spdyExecutor, spdyErr := remotecommand.NewSPDYExecutor(c.restConfig, "POST", execURL)

	var executor remotecommand.Executor
	if wsErr == nil && spdyErr == nil {
		executor, _ = remotecommand.NewFallbackExecutor(wsExecutor, spdyExecutor, func(err error) bool {
			return err != nil
		})
	} else if wsErr == nil {
		executor = wsExecutor
	} else if spdyErr == nil {
		executor = spdyExecutor
	} else {
		return "", fmt.Errorf("creating executor for pod %s: ws err: %v, spdy err: %v", podName, wsErr, spdyErr)
	}

	var stdout, stderr bytes.Buffer
	err := executor.StreamWithContext(ctx, remotecommand.StreamOptions{Stdout: &stdout, Stderr: &stderr})
	if err != nil {
		return "", fmt.Errorf("exec in pod %s failed (stderr: %s): %w", podName, stderr.String(), err)
	}
	return stdout.String(), nil
}

// parseServiceList parses the JSON output of `cilium-dbg service list -o json`
// and returns true if the given clusterIP has a service of type "LocalRedirect".
func (c *Checker) parseServiceList(jsonOutput string, clusterIP string) (bool, error) {
	var services []ciliumService
	err := json.Unmarshal([]byte(jsonOutput), &services)
	if err != nil {
		return false, fmt.Errorf("parsing cilium service list: %w", err)
	}
	for _, service := range services {
		if service.Spec.FrontendAddress.IP == clusterIP {
			fieldType := service.Spec.Flags["type"]
			if fieldType == "" {
				fieldType = service.Spec.Flags["Type"]
			}
			if fieldType == "LocalRedirect" {
				return true, nil
			}
		}
	}
	return false, nil
}
