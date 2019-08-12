// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"fmt"
	"io"
	"net/url"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.exec")

//go:generate mockgen -package mocks -destination mocks/remotecommand_mock.go k8s.io/client-go/tools/remotecommand Executor
type client struct {
	namespace               string
	clientset               kubernetes.Interface
	remoteCmdExecutorGetter func(method string, url *url.URL) (remotecommand.Executor, error)
	pipGetter               func() (io.Reader, io.WriteCloser)

	podGetter typedcorev1.PodInterface
}

// Executor provides the API to exec or cp on a pod inside the cluster.
type Executor interface {
	Exec(params ExecParams, cancel <-chan struct{}) error
	Copy(params CopyParam, cancel <-chan struct{}) error
}

// GetInClusterClient returns a in-cluster kubernetes clientset.
func GetInClusterClient() (kubernetes.Interface, *rest.Config, error) {
	// creates the in-cluster config.
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	// creates the clientset.
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return c, config, nil
}

// New contructs an executor.
// no cross model/namespace allowed.
func New(namespace string, clientset kubernetes.Interface, config *rest.Config) Executor {
	return new(
		namespace,
		clientset,
		config,
		remotecommand.NewSPDYExecutor,
		func() (io.Reader, io.WriteCloser) { return io.Pipe() },
	)
}

func new(
	namespace string,
	clientset kubernetes.Interface,
	config *rest.Config,
	remoteCMDNewer func(config *rest.Config, method string, url *url.URL) (remotecommand.Executor, error),
	pipGetter func() (io.Reader, io.WriteCloser),
) Executor {
	return &client{
		namespace: namespace,
		clientset: clientset,
		remoteCmdExecutorGetter: func(method string, url *url.URL) (remotecommand.Executor, error) {
			return remoteCMDNewer(config, method, url)
		},
		podGetter: clientset.CoreV1().Pods(namespace),
		pipGetter: pipGetter,
	}
}

// ExecParams holds all the necessary parameters for Exec.
type ExecParams struct {
	Commands      []string
	Env           []string
	PodName       string
	ContainerName string
	WorkingDir    string

	Stdin  io.Reader
	Stdout io.Writer
	Stderr io.Writer
}

func (ep *ExecParams) validate(podGetter typedcorev1.PodInterface) (err error) {
	if len(ep.Commands) == 0 {
		return errors.NotValidf("empty commands")
	}

	if ep.PodName, ep.ContainerName, err = getValidatedPodContainer(
		podGetter, ep.PodName, ep.ContainerName,
	); err != nil {
		return errors.Trace(err)
	}
	return nil
}

// Exec runs commands on a pod in the cluster.
func (c client) Exec(params ExecParams, cancel <-chan struct{}) error {
	if err := params.validate(c.podGetter); err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(c.exec(params, cancel))
}

func processEnv(env []string) string {
	out := ""
	for _, v := range env {
		out += fmt.Sprintf("export %s; ", v)
	}
	return out
}

func (c client) exec(opts ExecParams, cancel <-chan struct{}) error {
	cmd := ""
	if opts.WorkingDir != "" {
		cmd += fmt.Sprintf("cd %s; ", opts.WorkingDir)
	}
	if len(opts.Env) > 0 {
		cmd += processEnv(opts.Env)
	}
	cmd += fmt.Sprintf("%s; ", strings.Join(opts.Commands, " "))
	cmdArgs := []string{"sh", "-c", cmd}

	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(opts.PodName).
		Namespace(c.namespace).
		SubResource("exec").
		Param("container", opts.ContainerName).
		VersionedParams(&core.PodExecOptions{
			Container: opts.ContainerName,
			Command:   cmdArgs,
			Stdin:     opts.Stdin != nil,
			Stdout:    opts.Stdout != nil,
			Stderr:    opts.Stderr != nil,
			TTY:       false,
		}, scheme.ParameterCodec)

	executor, err := c.remoteCmdExecutorGetter("POST", req.URL())
	if err != nil {
		return errors.Trace(err)
	}

	errChan := make(chan error, 1)
	go func() {
		errChan <- executor.Stream(remotecommand.StreamOptions{
			Stdin:  opts.Stdin,
			Stdout: opts.Stdout,
			Stderr: opts.Stderr,
			Tty:    false,
		})
	}()
	select {
	case err := <-errChan:
		return errors.Trace(err)
	case <-cancel:
		return errors.New(fmt.Sprintf("exec cancelled: %v", opts))
	}
}

func parsePodName(podName string) (string, error) {
	err := errors.NotValidf("podName %q", podName)
	slice := strings.SplitN(podName, "/", 2)
	if len(slice) == 1 {
		podName = slice[0]
	} else if slice[0] != "pod" {
		return "", err
	} else {
		podName = slice[1]
	}
	if len(podName) == 0 {
		return "", err
	}
	return podName, nil
}

func getValidatedPodContainer(
	podGetter typedcorev1.PodInterface, podName, containerName string,
) (string, string, error) {
	var err error
	if podName, err = parsePodName(podName); err != nil {
		return "", "", errors.Trace(err)
	}
	var pod *core.Pod
	pod, err = podGetter.Get(podName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return "", "", errors.Trace(err)
		}
		logger.Debugf("no pod named %q found", podName)
		logger.Debugf("try get pod by UID for %q", podName)
		pods, err := podGetter.List(metav1.ListOptions{})
		// TODO(caas): remove getting pod by Id (a bit expensive) once we started to store podName in cloudContainer doc.
		if err != nil {
			return "", "", errors.Trace(err)
		}
		for _, v := range pods.Items {
			if string(v.GetUID()) == podName {
				p := v
				podName = p.GetName()
				pod = &p
				break
			}
		}
	}
	if pod == nil {
		return "", "", errors.NotFoundf("pod %q", podName)
	}

	if pod.Status.Phase == core.PodSucceeded || pod.Status.Phase == core.PodFailed {
		return "", "", errors.New(fmt.Sprintf(
			"cannot exec into a container in a completed pod; current phase is %s", pod.Status.Phase,
		))
	}

	checkContainerExists := func(name string) error {
		for _, c := range pod.Spec.Containers {
			if c.Name == name {
				return nil
			}
		}
		return errors.NotFoundf("container %q", name)
	}

	if containerName != "" {
		if err = checkContainerExists(containerName); err != nil {
			return "", "", errors.Trace(err)
		}
	} else {
		containerName = pod.Spec.Containers[0].Name
		logger.Debugf("choose first container %q to exec", containerName)
	}
	return podName, containerName, nil
}
