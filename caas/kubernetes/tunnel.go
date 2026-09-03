// Copyright 2020 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package kubernetes

import (
	"context"
	"fmt"
	"io"
	"math/rand"
	"net/http"
	"strconv"
	"sync"
	"time"

	"github.com/juju/errors"
	corev1 "k8s.io/api/core/v1"
	k8serrors "k8s.io/apimachinery/pkg/api/errors"
	meta "k8s.io/apimachinery/pkg/apis/meta/v1"
	"k8s.io/apimachinery/pkg/fields"
	"k8s.io/apimachinery/pkg/labels"
	"k8s.io/apimachinery/pkg/util/httpstream"
	"k8s.io/client-go/informers"
	"k8s.io/client-go/kubernetes"
	"k8s.io/client-go/kubernetes/scheme"
	"k8s.io/client-go/rest"
	"k8s.io/client-go/tools/cache"
	"k8s.io/client-go/tools/portforward"
	"k8s.io/client-go/transport/spdy"

	"github.com/juju/juju/caas/kubernetes/pod"
)

const (
	// ForwardPortTimeout is the duration for waiting for a pod to be ready.
	ForwardPortTimeout time.Duration = time.Minute * 10
)

type portForwarder interface {
	ForwardPorts() error
	GetPorts() ([]portforward.ForwardedPort, error)
}

type portForwarderFactory func(
	dialer httpstream.Dialer,
	addresses []string,
	ports []string,
	stopChan <-chan struct{},
	readyChan chan struct{},
	out, errOut io.Writer,
) (portForwarder, error)

func defaultPortForwarder(
	dialer httpstream.Dialer,
	addresses []string,
	ports []string,
	stopChan <-chan struct{},
	readyChan chan struct{},
	out, errOut io.Writer,
) (portForwarder, error) {
	return portforward.NewOnAddresses(dialer, addresses, ports, stopChan, readyChan, out, errOut)
}

// Tunnel represents an ssh like tunnel to a Kubernetes Pod or Service
type Tunnel struct {
	client           rest.Interface
	config           *rest.Config
	Kind             TunnelKind
	errChan          chan error
	newPortForwarder portForwarderFactory
	LocalPort        string
	Namespace        string
	Out              io.Writer
	broken           chan struct{}
	closeOnce        sync.Once
	readyChan        chan struct{}
	RemotePort       string
	stopChan         chan struct{}
	Target           string
}

type TunnelKind string

const (
	TunnelKindPods     = TunnelKind("pods")
	TunnelKindServices = TunnelKind("services")
)

// Close disconnects a tunnel connection
func (t *Tunnel) Close() {
	t.closeOnce.Do(func() {
		close(t.stopChan)
	})
}

// ForwardError returns a port-forwarding error observed after the tunnel was
// reported as ready.
func (t *Tunnel) ForwardError() error {
	if t.errChan == nil {
		return nil
	}
	select {
	case err := <-t.errChan:
		return err
	default:
		return nil
	}
}

// Broken returns a channel that is closed when the port-forward tunnel is
// broken (the underlying forward goroutine exits).
func (t *Tunnel) Broken() <-chan struct{} {
	return t.broken
}

// findSuitablePodForService when tunneling to a kubernetes service we need to
// introspection.
func (t *Tunnel) findSuitablePodForService(ctx context.Context) (*corev1.Pod, error) {
	clientSet := kubernetes.New(t.client)
	service, err := clientSet.CoreV1().Services(t.Namespace).
		Get(ctx, t.Target, meta.GetOptions{})
	if k8serrors.IsNotFound(err) {
		return nil, errors.NewNotFound(err, "can't find service "+t.Target)
	} else if err != nil {
		return nil, errors.Trace(err)
	}

	pods, err := clientSet.CoreV1().Pods(t.Namespace).
		List(ctx, meta.ListOptions{
			LabelSelector: labels.SelectorFromSet(service.Spec.Selector).String(),
		})

	if err != nil {
		return nil, errors.Trace(err)
	}

	podCount := len(pods.Items)
	if podCount == 0 {
		return nil, errors.NotFoundf("pods for service %s", t.Target)
	} else if podCount == 1 {
		return &pods.Items[0], nil
	}

	return &pods.Items[rand.Intn(podCount-1)], nil
}

// ForwardPort starts forwarding RemotePort to an assigned IPv4 localhost port.
// A Tunnel is single-use for forwarding; call ForwardPort at most once, then
// Close to stop the forwarding session.
func (t *Tunnel) ForwardPort(ctx context.Context) error {
	if !t.IsValidTunnelKind() {
		return fmt.Errorf("invalid tunnel kind %s", t.Kind)
	}

	ctx, cancelFunc := context.WithTimeout(ctx, ForwardPortTimeout)
	defer cancelFunc()

	podName := t.Target

	if t.Kind == TunnelKindServices {
		pod, err := t.findSuitablePodForService(ctx)
		if err != nil {
			return errors.Trace(err)
		}
		podName = pod.Name
	}

	err := t.waitForPodReady(ctx, podName)
	if err != nil {
		return errors.Annotatef(err, "waiting for pod %s to become ready for tunnel", podName)
	}

	u := t.client.Post().
		Resource(string(TunnelKindPods)).
		Namespace(t.Namespace).
		Name(podName).
		SubResource("portforward").URL()

	transport, upgrader, err := spdy.RoundTripperFor(t.config)
	if err != nil {
		return err
	}
	dialer := spdy.NewDialer(upgrader, &http.Client{Transport: transport}, http.MethodPost, u)

	return t.forwardPort(ctx, dialer, podName)
}

func (t *Tunnel) forwardPort(ctx context.Context, dialer httpstream.Dialer, podName string) error {
	ports := []string{fmt.Sprintf("0:%s", t.RemotePort)}
	pf, err := t.newPortForwarder(
		dialer,
		[]string{"127.0.0.1"},
		ports,
		t.stopChan,
		t.readyChan,
		t.Out,
		t.Out,
	)
	if err != nil {
		return err
	}

	t.errChan = make(chan error, 1)
	go func() {
		t.errChan <- pf.ForwardPorts()
		close(t.broken)
	}()

	// Start a pod health check that closes the tunnel if the pod is
	// deleted. The port-forward SPDY connection to the API server may
	// stay alive even after the pod is gone, so ForwardPorts() may
	// never return on its own.
	t.startPodHealthCheck(ctx, podName)

	select {
	case <-ctx.Done():
		t.Close()
		return ctx.Err()
	case err = <-t.errChan:
		t.Close()
		return fmt.Errorf("forwarding ports: %v", err)
	case <-t.readyChan:
		forwardedPorts, err := pf.GetPorts()
		if err != nil {
			t.Close()
			return fmt.Errorf("getting forwarded ports: %w", err)
		}
		if len(forwardedPorts) == 0 {
			t.Close()
			return errors.New("no forwarded ports")
		}
		t.LocalPort = strconv.Itoa(int(forwardedPorts[0].Local))
		return nil
	}
}

// startPodHealthCheck watches the pod via the Kubernetes API and closes the
// tunnel if the pod is deleted or no longer running. The SPDY connection to
// the API server may persist after the pod is gone, so this provides an
// independent signal that the tunnel is broken.
func (t *Tunnel) startPodHealthCheck(ctx context.Context, podName string) {
	if t.client == nil {
		return
	}
	t.defaultPodHealthCheck(ctx, podName)
}

// defaultPodHealthCheck sets up a pod health check that closes the tunnel if
// the pod is deleted or no longer running. The SPDY connection to the API
// server may persist after the pod is gone, so this provides an independent
// signal that the tunnel is broken.
const podHealthCheckResyncPeriod = 10 * time.Second

func (t *Tunnel) defaultPodHealthCheck(ctx context.Context, podName string) {
	clientSet := kubernetes.New(t.client)
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientSet,
		podHealthCheckResyncPeriod,
		informers.WithNamespace(t.Namespace),
		informers.WithTweakListOptions(func(options *meta.ListOptions) {
			options.FieldSelector = fields.OneTermEqualSelector("metadata.name", podName).String()
		}),
	)
	informer := factory.Core().V1().Pods().Informer()

	// ForwardPort's context is cancelled once the local port is ready, while
	// the health check must run for the tunnel's entire lifetime.
	healthCtx, cancel := context.WithCancel(context.Background())
	go func() {
		<-t.stopChan
		cancel()
	}()

	reg, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			p, ok := obj.(*corev1.Pod)
			if !ok {
				return
			}
			if p.Name == podName && !pod.IsPodRunning(p) {
				logger.Debugf(ctx, "tunnel pod %s not running, closing tunnel", podName)
				t.Close()
			}
		},
		DeleteFunc: func(obj any) {
			key, err := cache.DeletionHandlingMetaNamespaceKeyFunc(obj)
			if err != nil {
				logger.Errorf(ctx, "getting deleted tunnel pod key: %v", err)
				return
			}
			if key == t.Namespace+"/"+podName {
				logger.Debugf(ctx, "tunnel pod %s deleted, closing tunnel", podName)
				t.Close()
			}
		},
		UpdateFunc: func(_, newObj any) {
			p, ok := newObj.(*corev1.Pod)
			if !ok {
				return
			}
			if p.Name == podName && !pod.IsPodRunning(p) {
				logger.Debugf(ctx, "tunnel pod %s not running, closing tunnel", podName)
				t.Close()
			}
		},
	})
	if err != nil {
		logger.Errorf(ctx, "failed to add pod health check handler: %v", err)
		return
	}

	err = informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		if !errors.Is(err, context.Canceled) {
			logger.Errorf(ctx, "pod health check watch error: %v", err)
		}
	})
	if err != nil {
		logger.Errorf(ctx, "failed to set pod health check error handler: %v", err)
		return
	}

	go func() {
		informer.RunWithContext(healthCtx)
		_ = informer.RemoveEventHandler(reg)
	}()

	go func() {
		if !cache.WaitForCacheSync(healthCtx.Done(), informer.HasSynced) {
			return
		}
		_, exists, err := informer.GetStore().GetByKey(t.Namespace + "/" + podName)
		if err != nil {
			logger.Errorf(ctx, "checking tunnel pod %s health: %v", podName, err)
			return
		}
		if !exists {
			logger.Debugf(ctx, "tunnel pod %s deleted, closing tunnel", podName)
			t.Close()
		}
	}()
}

// IsValidTunnelKind tests that the tunnel kind supplied to this tunnel is valid
func (t *Tunnel) IsValidTunnelKind() bool {
	switch t.Kind {
	case TunnelKindPods,
		TunnelKindServices:
		return true
	}
	return false
}

// NewTunnelForConfig constructs a new tunnel from the provided rest config
func NewTunnelForConfig(
	c *rest.Config,
	kind TunnelKind,
	namespace,
	target,
	remotePort string,
) (*Tunnel, error) {

	config := *c
	gv := corev1.SchemeGroupVersion
	config.GroupVersion = &gv
	config.APIPath = "/api"
	config.NegotiatedSerializer = scheme.Codecs.WithoutConversion()

	client, err := rest.RESTClientFor(&config)
	if err != nil {
		return nil, fmt.Errorf("failed creating kubernetes rest client for tunnel: %w", err)
	}

	return NewTunnel(client, &config, kind, namespace, target, remotePort), nil
}

// NewTunnel constructs a new kubernetes tunnel for the provided information
func NewTunnel(
	client rest.Interface,
	c *rest.Config,
	kind TunnelKind,
	namespace,
	target,
	remotePort string) *Tunnel {

	return &Tunnel{
		client:           client,
		config:           c,
		Kind:             kind,
		newPortForwarder: defaultPortForwarder,
		Namespace:        namespace,
		Out:              io.Discard,
		broken:           make(chan struct{}),
		readyChan:        make(chan struct{}, 1),
		RemotePort:       remotePort,
		stopChan:         make(chan struct{}, 1),
		Target:           target,
	}
}

// waitForPodReady waits for the provided pod name relative to this tunnels
// namespace to become fully ready in the pod conditions. This func will block
// until the pod is ready of the context dead line has fired.
func (t *Tunnel) waitForPodReady(ctx context.Context, podName string) error {
	clientSet := kubernetes.New(t.client)
	factory := informers.NewSharedInformerFactoryWithOptions(
		clientSet,
		time.Minute,
		informers.WithNamespace(t.Namespace),
	)
	informer := factory.Core().V1().Pods().Informer()

	eventChan := make(chan error)
	waitCtx, cancel := context.WithCancel(ctx)
	defer func() {
		cancel()
		close(eventChan)
	}()

	reg, err := informer.AddEventHandler(cache.ResourceEventHandlerFuncs{
		AddFunc: func(obj any) {
			objPod, valid := obj.(*corev1.Pod)
			if !valid {
				select {
				case <-waitCtx.Done():
				case eventChan <- errors.New("expected valid pod for informer"):
				}
				return
			}

			if objPod.Name == podName && pod.IsPodRunning(objPod) {
				select {
				case <-waitCtx.Done():
				case eventChan <- nil:
				}
			}
		},
		UpdateFunc: func(oldObj, newObj any) {
			objPod, valid := newObj.(*corev1.Pod)
			if !valid {
				select {
				case <-waitCtx.Done():
				case eventChan <- errors.New("expected valid pod for informer"):
				}
				return
			}

			if objPod.Name == podName && pod.IsPodRunning(objPod) {
				select {
				case <-waitCtx.Done():
				case eventChan <- nil:
				}
			}
		},
		DeleteFunc: func(obj any) {
			pod, valid := obj.(*corev1.Pod)
			if !valid {
				select {
				case <-waitCtx.Done():
				case eventChan <- errors.New("expected valid pod for informer"):
				}
				return
			}

			if pod.Name == podName {
				select {
				case <-waitCtx.Done():
				case eventChan <- errors.Errorf("tunnel pod %s is being deleted", podName):
				}
			}
		},
	})
	if err != nil {
		return errors.Trace(err)
	}
	defer func() {
		_ = informer.RemoveEventHandler(reg)
	}()

	err = informer.SetWatchErrorHandler(func(r *cache.Reflector, err error) {
		if !errors.Is(err, context.Canceled) {
			logger.Errorf(ctx, "error watching pod %s: %v", podName, err)
		}
	})
	if err != nil {
		return errors.Trace(err)
	}

	go informer.RunWithContext(waitCtx)

	select {
	case <-ctx.Done():
		return ctx.Err()
	case err := <-eventChan:
		return err
	}
}
