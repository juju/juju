// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package provider

import (
	"crypto/rand"
	"fmt"
	"io"

	"github.com/juju/errors"
	v1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
)

// Constants below are copied from "k8s.io/kubernetes/pkg/kubelet/images"
// to avoid introducing the huge dependency.
var (
	// errImagePullBackOff - Container image pull failed, kubelet is backing off image pull
	errImagePullBackOff = "ImagePullBackOff"
	// errImageInspect - Unable to inspect image
	errImageInspect = "ImageInspectError"
	// errImagePull - General image pull error
	errImagePull = "ErrImagePull"
	// errImageNeverPull - Required Image is absent on host and PullPolicy is NeverPullImage
	errImageNeverPull = "ErrImageNeverPull"
	// errRegistryUnavailable - Get http error when pulling image from registry
	errRegistryUnavailable = "RegistryUnavailable"
	// errInvalidImageName - Unable to parse the image name.
	errInvalidImageName = "InvalidImageName"
)

func (k *kubernetesClient) operatorImagePrepullCheck(image string) error {
	podsAPI := k.client().CoreV1().Pods(k.namespace)
	var randSuffix [4]byte
	if _, err := io.ReadFull(rand.Reader, randSuffix[0:4]); err != nil {
		return errors.Trace(err)
	}
	name := fmt.Sprintf("operator-image-prepull-%x", randSuffix)
	pod := &v1.Pod{
		ObjectMeta: metav1.ObjectMeta{
			Name:      name,
			Namespace: k.namespace,
		},
		Spec: v1.PodSpec{
			RestartPolicy: v1.RestartPolicyNever,
			Containers: []v1.Container{
				v1.Container{
					Name:            "jujud",
					Image:           image,
					ImagePullPolicy: v1.PullIfNotPresent,
					Command:         []string{"/opt/jujud"},
					Args:            []string{"version"},
				},
			},
		},
	}

	logger.Debugf("creating temporary pod %s to validate image %s", name, image)

	w, err := k.watchPod(name)
	if err != nil {
		return errors.Trace(err)
	}
	defer w.Kill()
	pod, err = podsAPI.Create(pod)
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		err := podsAPI.Delete(name, &metav1.DeleteOptions{})
		if err != nil && !k8serrors.IsNotFound(err) {
			logger.Errorf("failed to delete temporary pod %s: %v", name, err)
		}
	}()

	for {
		switch pod.Status.Phase {
		case v1.PodPending:
			if hasImagePullFailure(pod) {
				return errors.NotFoundf("image not pullable")
			}
		case v1.PodRunning:
		case v1.PodSucceeded:
			// Image exists.
			// TODO(caas): return operator version information from stdout.
			return nil
		case v1.PodFailed:
			return errors.Errorf("pod failed with reason %s", pod.Status.Reason)
		case v1.PodUnknown:
		}
		<-w.Changes()
		pod, err = podsAPI.Get(name, metav1.GetOptions{IncludeUninitialized: true})
		if err != nil {
			return errors.Annotatef(err, "failed to get pod")
		}
	}
}

func hasImagePullFailure(pod *v1.Pod) bool {
	containerStatus := append(pod.Status.InitContainerStatuses, pod.Status.ContainerStatuses...)
	for _, c := range containerStatus {
		waiting := c.State.Waiting
		if waiting == nil {
			continue
		}
		logger.Debugf("pod %s waiting for container %s due to %s %s",
			pod.Name, c.Name, waiting.Reason, waiting.Message)
		switch waiting.Reason {
		case errImagePullBackOff:
			return true
		case errImageInspect:
			return true
		case errImagePull:
			// Could just be a transient error, wait for
			return false
		case errImageNeverPull:
			return true
		case errRegistryUnavailable:
			// Could just be a transient error
			return false
		case errInvalidImageName:
			return true
		}
	}
	return false
}
