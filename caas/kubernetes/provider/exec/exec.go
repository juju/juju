// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"bytes"
	"fmt"
	"io"
	"net/url"
	"strings"
	"syscall"
	"time"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	"github.com/juju/utils"
	"github.com/kballard/go-shellquote"
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

const (
	sigkillRetryDelay = 100 * time.Millisecond
	gracefulKillDelay = 10 * time.Second
	maxTrys           = 10
)

var randomString = utils.RandomString

//go:generate go run github.com/golang/mock/mockgen -package mocks -destination mocks/remotecommand_mock.go k8s.io/client-go/tools/remotecommand Executor
type client struct {
	namespace               string
	clientset               kubernetes.Interface
	remoteCmdExecutorGetter func(method string, url *url.URL) (remotecommand.Executor, error)
	pipGetter               func() (io.Reader, io.WriteCloser)

	podGetter typedcorev1.PodInterface
}

// Executor provides the API to exec or cp on a pod inside the cluster.
type Executor interface {
	Status(params StatusParams) (*Status, error)
	Exec(params ExecParams, cancel <-chan struct{}) error
	Copy(params CopyParams, cancel <-chan struct{}) error
}

// NewInCluster returns a in-cluster exec client.
func NewInCluster(namespace string) (Executor, error) {
	// creates the in-cluster config.
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, errors.Trace(err)
	}
	// creates the clientset.
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, errors.Trace(err)
	}

	return New(namespace, c, config), nil
}

// New contructs an executor.
// no cross model/namespace allowed.
func New(namespace string, clientset kubernetes.Interface, config *rest.Config) Executor {
	return newClient(
		namespace,
		clientset,
		config,
		remotecommand.NewSPDYExecutor,
		func() (io.Reader, io.WriteCloser) { return io.Pipe() },
	)
}

func newClient(
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

	Signal <-chan syscall.Signal
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

func processEnv(env []string) (string, error) {
	out := ""
	for _, s := range env {
		values := strings.SplitN(s, "=", 2)
		if len(values) != 2 {
			return "", errors.NotValidf("env %q", s)
		}
		key := values[0]
		value := values[1]
		out += fmt.Sprintf("export %s=%s; ", key, shellquote.Join(value))
	}
	return out, nil
}

func (c client) exec(opts ExecParams, cancel <-chan struct{}) error {
	pidFile := fmt.Sprintf("/tmp/%s.pid", randomString(8, utils.LowerAlpha))
	cmd := ""
	if opts.WorkingDir != "" {
		cmd += fmt.Sprintf("cd %s; ", opts.WorkingDir)
	}
	if len(opts.Env) > 0 {
		env, err := processEnv(opts.Env)
		if err != nil {
			return errors.Trace(err)
		}
		cmd += env
	}
	cmd += fmt.Sprintf("mkdir -p /tmp; echo $$ > %s; ", pidFile)
	cmd += fmt.Sprintf("exec sh -c %s; ", shellquote.Join(strings.Join(opts.Commands, " ")))
	cmdArgs := []string{"sh", "-c", cmd}
	logger.Debugf("exec on pod %q for cmd %+q", opts.PodName, cmdArgs)
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

	sendSignal := func(sig syscall.Signal, group bool) error {
		var cmd []string
		if group {
			// Group is mostly for SIGKILL sending a signal to the whole process group.
			cmd = []string{"sh", "-c", fmt.Sprintf("kill -%d -$(cat %s)", int(sig), pidFile)}
		} else {
			cmd = []string{"sh", "-c", fmt.Sprintf("kill -%d $(cat %s)", int(sig), pidFile)}
		}
		req := c.clientset.CoreV1().RESTClient().Post().
			Resource("pods").
			Name(opts.PodName).
			Namespace(c.namespace).
			SubResource("exec").
			Param("container", opts.ContainerName).
			VersionedParams(&core.PodExecOptions{
				Container: opts.ContainerName,
				Command:   cmd,
				Stdout:    true,
				Stderr:    true,
				TTY:       false,
			}, scheme.ParameterCodec)
		executor, err := c.remoteCmdExecutorGetter("POST", req.URL())
		if err != nil {
			return errors.Trace(err)
		}
		out := &bytes.Buffer{}
		err = executor.Stream(remotecommand.StreamOptions{
			Stdout: out,
			Stderr: out,
			Tty:    false,
		})
		if exitErr, ok := err.(ExitError); ok {
			// Ignore exitcode from kill, as the process may have already exited or
			// the pid file hasn't yet been written.
			logger.Debugf("%q exited with code %d", strings.Join(cmd, " "), exitErr.ExitStatus())
			return nil
		}
		return err
	}

	kill := make(chan struct{}, 1)
	killTrys := 0
	var timer <-chan time.Time
	for {
		select {
		case err := <-errChan:
			return errors.Trace(err)
		case <-cancel:
			cancel = nil
			err := sendSignal(syscall.SIGTERM, false)
			if err != nil {
				return errors.Annotatef(err, "send signal %d failed", syscall.SIGTERM)
			}
			// Trigger SIGKILL
			timer = time.After(gracefulKillDelay)
		case <-kill:
			killTrys++
			if killTrys > maxTrys {
				return errors.Errorf("SIGKILL failed after %d attempts", maxTrys)
			}
			err := sendSignal(syscall.SIGKILL, true)
			if err != nil {
				return errors.Annotatef(err, "send signal %d failed", syscall.SIGKILL)
			}
			// Retry SIGKILL.
			timer = time.After(sigkillRetryDelay)
		case <-timer:
			timer = nil
			// Trigger SIGKILL
			select {
			case kill <- struct{}{}:
			default:
			}
		case sig := <-opts.Signal:
			if sig == syscall.SIGKILL {
				// Trigger SIGKILL
				select {
				case kill <- struct{}{}:
				default:
				}
			} else {
				err := sendSignal(sig, false)
				if err != nil {
					return errors.Annotatef(err, "send signal %d failed", sig)
				}
			}
		}
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

func getValidatedPod(podGetter typedcorev1.PodInterface, podName string) (*core.Pod, error) {
	var err error
	if podName, err = parsePodName(podName); err != nil {
		return nil, errors.Trace(err)
	}
	var pod *core.Pod
	pod, err = podGetter.Get(podName, metav1.GetOptions{})
	if err != nil {
		if !k8serrors.IsNotFound(err) {
			return nil, errors.Trace(err)
		}
		logger.Debugf("no pod named %q found", podName)
		logger.Debugf("try get pod by UID for %q", podName)
		pods, err := podGetter.List(metav1.ListOptions{})
		// TODO(caas): remove getting pod by Id (a bit expensive) once we started to store podName in cloudContainer doc.
		if err != nil {
			return nil, errors.Trace(err)
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
		return nil, errors.NotFoundf("pod %q", podName)
	}
	return pod, nil
}

func getValidatedPodContainer(
	podGetter typedcorev1.PodInterface, podName, containerName string,
) (string, string, error) {
	pod, err := getValidatedPod(podGetter, podName)
	if err != nil {
		return "", "", errors.Trace(err)
	}

	checkContainerExists := func(name string) error {
		for _, c := range pod.Spec.InitContainers {
			if c.Name == name {
				return nil
			}
		}
		for _, c := range pod.Spec.Containers {
			if c.Name == name {
				return nil
			}
		}
		return errors.NotFoundf("container %q", name)
	}

	var containerStatus []core.ContainerStatus
	switch pod.Status.Phase {
	case core.PodPending:
		containerStatus = pod.Status.InitContainerStatuses
	case core.PodRunning:
		containerStatus = pod.Status.ContainerStatuses
	default:
		return "", "", errors.New(fmt.Sprintf(
			"cannot exec into a container within a %s pod", pod.Status.Phase,
		))
	}

	if containerName != "" {
		if err = checkContainerExists(containerName); err != nil {
			return "", "", errors.Trace(err)
		}
	} else {
		containerName = pod.Spec.Containers[0].Name
		logger.Debugf("choose first container %q to exec", containerName)
	}

	matchContainerStatus := func(name string) (*core.ContainerStatus, error) {
		for _, status := range containerStatus {
			if status.Name == name {
				return &status, nil
			}
		}
		return nil, containerNotRunningError(name)
	}

	status, err := matchContainerStatus(containerName)
	if err != nil {
		return "", "", errors.Trace(err)
	}
	if status.State.Running == nil {
		return "", "", containerNotRunningError(containerName)
	}

	return pod.Name, containerName, nil
}
