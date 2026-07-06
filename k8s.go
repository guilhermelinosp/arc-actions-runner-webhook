package main

import (
	"context"
	"fmt"

	"k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/apis/meta/v1/unstructured"
	"k8s.io/apimachinery/pkg/runtime/schema"
	"k8s.io/client-go/dynamic"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/util/retry"
)

var (
	runnerGVR = schema.GroupVersionResource{
		Group:    "actions.summerwind.dev",
		Version:  "v1alpha1",
		Resource: "runnerdeployments",
	}
	hraGVR = schema.GroupVersionResource{
		Group:    "actions.summerwind.dev",
		Version:  "v1alpha1",
		Resource: "horizontalrunnerautoscalers",
	}
)

type k8sController struct {
	dynClient dynamic.Interface
	k8sClient kubernetes.Interface
	namespace string
	runnerImg string
}

func (kc *k8sController) runnerExists(ctx context.Context, fullName string) (bool, error) {
	list, err := kc.dynClient.Resource(runnerGVR).Namespace(kc.namespace).List(ctx, metav1.ListOptions{})
	if err != nil {
		return false, fmt.Errorf("list runnerdeployments: %w", err)
	}
	for _, item := range list.Items {
		repo, _, _ := unstructured.NestedString(item.Object, "spec", "template", "spec", "repository")
		if repo == fullName {
			return true, nil
		}
	}
	return false, nil
}

func (kc *k8sController) createRunner(ctx context.Context, fullName, repoName string) error {
	safeName := sanitize(repoName)

	manifestRD := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "actions.summerwind.dev/v1alpha1",
			"kind":       "RunnerDeployment",
			"metadata": map[string]interface{}{
				"name":      "runner-" + safeName,
				"namespace": kc.namespace,
			},
			"spec": map[string]interface{}{
				"replicas": 1,
				"template": map[string]interface{}{
					"spec": map[string]interface{}{
						"repository":                   fullName,
						"image":                        kc.runnerImg,
						"dockerdWithinRunnerContainer": false,
						"labels":                       []interface{}{"arc-runner"},
						"resources": map[string]interface{}{
							"limits": map[string]interface{}{
								"cpu":    "1",
								"memory": "2Gi",
							},
							"requests": map[string]interface{}{
								"cpu":    "100m",
								"memory": "256Mi",
							},
						},
					},
				},
			},
		},
	}

	manifestHRA := &unstructured.Unstructured{
		Object: map[string]interface{}{
			"apiVersion": "actions.summerwind.dev/v1alpha1",
			"kind":       "HorizontalRunnerAutoscaler",
			"metadata": map[string]interface{}{
				"name":      "runner-" + safeName + "-autoscaler",
				"namespace": kc.namespace,
			},
			"spec": map[string]interface{}{
				"scaleTargetRef": map[string]interface{}{
					"name": "runner-" + safeName,
					"kind": "RunnerDeployment",
				},
				"minReplicas": 0,
				"maxReplicas": 5,
				"metrics": []interface{}{
					map[string]interface{}{
						"type": "TotalNumberOfQueuedAndInProgressWorkflowRuns",
						"repositoryNames": []interface{}{fullName},
					},
				},
			},
		},
	}

	// Retry com backoff
	err := retry.OnError(retry.DefaultRetry, func(err error) bool { return true }, func() error {
		_, err := kc.dynClient.Resource(runnerGVR).Namespace(kc.namespace).
			Create(ctx, manifestRD, metav1.CreateOptions{})
		if err != nil && !errors.IsAlreadyExists(err) {
			return err
		}
		_, err2 := kc.dynClient.Resource(hraGVR).Namespace(kc.namespace).
			Create(ctx, manifestHRA, metav1.CreateOptions{})
		if err2 != nil && !errors.IsAlreadyExists(err2) {
			return err2
		}
		return nil
	})
	if err != nil {
		return fmt.Errorf("create resources: %w", err)
	}

	return nil
}

func sanitize(name string) string {
	result := make([]byte, 0, len(name))
	for _, c := range []byte(name) {
		if (c >= 'a' && c <= 'z') || (c >= '0' && c <= '9') || c == '-' {
			result = append(result, c)
		} else if c >= 'A' && c <= 'Z' {
			result = append(result, c+32) // tolower
		} else {
			result = append(result, '-')
		}
	}
	return string(result)
}
