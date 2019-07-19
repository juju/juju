// Copyright 2019 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package exec

import (
	"fmt"
	"io"
	"strings"

	"github.com/juju/errors"
	"github.com/juju/loggo"
	core "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	metav1 "k8s.io/apimachinery/pkg/apis/meta/v1"
	// "k8s.io/apimachinery/pkg/runtime"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	typedcorev1 "k8s.io/client-go/kubernetes/typed/core/v1"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/remotecommand"
)

var logger = loggo.GetLogger("juju.kubernetes.provider.exec")

// // parameterCodec handles versioning of objects that are converted to query parameters.
// var parameterCodec = runtime.NewParameterCodec(runtime.NewScheme())

type client struct {
	namesapce string
	clientset kubernetes.Interface
	config    *rest.Config
}

// Executer provides the API to exec or cp on a pod inside the cluster.
type Executer interface {
	Exec(params ExecParams) error
	Copy(from, to string) error
}

// GetInClusterClient returns a in-cluster kubernetes clientset.
func GetInClusterClient() (kubernetes.Interface, *rest.Config, error) {
	// creates the in-cluster config
	config, err := rest.InClusterConfig()
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	// creates the clientset
	c, err := kubernetes.NewForConfig(config)
	if err != nil {
		return nil, nil, errors.Trace(err)
	}
	return c, config, nil
}

// New contructs an executer.
func New(namesapce string, clientset kubernetes.Interface, config *rest.Config) Executer {
	return &client{namesapce, clientset, config}
}

func (c client) Exec(params ExecParams) error {
	opts, err := c.execParamsToOptions(params)
	if err != nil {
		return errors.Trace(err)
	}
	return errors.Trace(c.exec(opts))
}

func (c client) execParamsToOptions(params ExecParams) (*execOptions, error) {
	opts := &execOptions{podGetter: c.clientset.CoreV1().Pods(c.namesapce)}
	opts.PodName = params.PodName
	opts.Commands = params.Commands
	opts.Env = params.Env
	opts.Stdin = params.Stdin
	opts.Stdout = params.Stdout
	opts.Stderr = params.Stderr
	// opts.podGetter = c.clientset.CoreV1().Pods(c.namesapce)
	if err := opts.validate(); err != nil {
		return nil, errors.Trace(err)
	}
	return opts, nil
}

func processEnv(env []string) string {
	out := ""
	for _, v := range env {
		out += fmt.Sprintf("export %s; ", v)
	}
	return out
}

func (c client) exec(opts *execOptions) error {
	cmd := ""
	if opts.WorkingDir != "" {
		cmd += fmt.Sprintf("cd %s; ", opts.WorkingDir)
	}
	if len(opts.Env) > 0 {
		cmd += processEnv(opts.Env)
	}
	cmd += fmt.Sprintf("%s; ", strings.Join(opts.Commands, " "))
	cmdArgs := []string{"sh", "-c", cmd}

	logger.Criticalf("exec opts -> %+v", opts)
	logger.Criticalf("cmdArgs -> %+v", cmdArgs)
	req := c.clientset.CoreV1().RESTClient().Post().
		Resource("pods").
		Name(opts.PodName).
		Namespace(c.namesapce).
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

	executer, err := remotecommand.NewSPDYExecutor(c.config, "POST", req.URL())
	logger.Criticalf("req.URL() -> %+v", req.URL())
	if err != nil {
		return errors.Trace(err)
	}
	err = executer.Stream(remotecommand.StreamOptions{
		Stdin:  opts.Stdin,
		Stdout: opts.Stdout,
		Stderr: opts.Stderr,
		Tty:    false,
	})
	return errors.Trace(err)
}

func (c client) Copy(from, to string) error {
	// TODO: !!!!!!!!!!!!!
	return nil
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

type execOptions struct {
	ExecParams

	pod       *core.Pod
	podGetter typedcorev1.PodInterface
}

func (op *execOptions) parsePodName() error {
	err := errors.NotValidf("podName %q", op.PodName)
	slice := strings.SplitN(op.PodName, "/", 2)
	if len(slice) == 1 {
		op.PodName = slice[0]
	} else if slice[0] != "pod" {
		return err
	} else {
		op.PodName = slice[1]
	}
	if len(op.PodName) == 0 {
		return err
	}
	return nil
}

func (op *execOptions) validatePodContainer() (err error) {
	if err = op.parsePodName(); err != nil {
		return errors.Trace(err)
	}
	if op.pod, err = op.podGetter.Get(op.PodName, metav1.GetOptions{}); err != nil {
		if k8serrors.IsNotFound(err) {
			return errors.NotFoundf("pod %q", op.PodName)
		}
		return errors.Trace(err)
	}

	if op.pod.Status.Phase == core.PodSucceeded || op.pod.Status.Phase == core.PodFailed {
		return errors.New(fmt.Sprintf("cannot exec into a container in a completed pod; current phase is %s", op.pod.Status.Phase))
	}

	checkContainerExists := func(name string) error {
		for _, c := range op.pod.Spec.Containers {
			if c.Name == name {
				return nil
			}
		}
		return errors.NotFoundf("container %q", name)
	}

	if op.ContainerName != "" {
		if err = checkContainerExists(op.ContainerName); err != nil {
			return errors.Trace(err)
		}
	} else {
		op.ContainerName = op.pod.Spec.Containers[0].Name
		logger.Debugf("choose first container %q to exec", op.ContainerName)
	}
	return nil
}

func (op *execOptions) validate() error {
	if len(op.Commands) == 0 {
		return errors.NotValidf("commands %v", op.Commands)
	}

	if err := op.validatePodContainer(); err != nil {
		return errors.Trace(err)
	}
	return nil
}
