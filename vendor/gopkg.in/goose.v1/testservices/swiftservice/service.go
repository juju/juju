// Swift double testing service - internal direct API implementation

package swiftservice

import (
	"fmt"
	"net/url"
	"sort"
	"strings"
	"sync"
	"time"

	"gopkg.in/goose.v1/swift"
	"gopkg.in/goose.v1/testservices"
	"gopkg.in/goose.v1/testservices/identityservice"
)

type object map[string][]byte

var _ testservices.HttpService = (*Swift)(nil)
var _ identityservice.ServiceProvider = (*Swift)(nil)

type Swift struct {
	testservices.ServiceInstance

	mu         sync.Mutex // protects the remaining fields
	containers map[string]object
}

// New creates an instance of the Swift object, given the parameters.
func New(hostURL, versionPath, tenantId, region string, identityService, fallbackIdentity identityservice.IdentityService) *Swift {
	URL, err := url.Parse(hostURL)
	if err != nil {
		panic(err)
	}
	hostname := URL.Host
	if !strings.HasSuffix(hostname, "/") {
		hostname += "/"
	}
	swift := &Swift{
		containers: make(map[string]object),
		ServiceInstance: testservices.ServiceInstance{
			IdentityService:         identityService,
			FallbackIdentityService: fallbackIdentity,
			Scheme:                  URL.Scheme,
			Hostname:                hostname,
			VersionPath:             versionPath,
			TenantId:                tenantId,
			Region:                  region,
		},
	}
	if identityService != nil {
		identityService.RegisterServiceProvider("swift", "object-store", swift)
	}
	return swift
}

func (s *Swift) endpointURL(path string) string {
	ep := s.Scheme + "://" + s.Hostname + s.VersionPath + "/" + s.TenantId
	if path != "" {
		ep += "/" + strings.TrimLeft(path, "/")
	}
	return ep
}

func (s *Swift) Endpoints() []identityservice.Endpoint {
	ep := identityservice.Endpoint{
		AdminURL:    s.endpointURL(""),
		InternalURL: s.endpointURL(""),
		PublicURL:   s.endpointURL(""),
		Region:      s.Region,
	}
	return []identityservice.Endpoint{ep}
}

func (s *Swift) V3Endpoints() []identityservice.V3Endpoint {
	url := s.endpointURL("")
	return identityservice.NewV3Endpoints(url, url, url, s.RegionID)
}

// HasContainer verifies the given container exists or not.
func (s *Swift) HasContainer(name string) bool {
	s.mu.Lock()
	_, ok := s.containers[name]
	s.mu.Unlock()
	return ok
}

// GetObject retrieves a given object from its container, returning
// the object data or an error.
func (s *Swift) GetObject(container, name string) ([]byte, error) {
	if err := s.ProcessFunctionHook(s, container, name); err != nil {
		return nil, err
	}
	s.mu.Lock()
	data, ok := s.containers[container][name]
	s.mu.Unlock()
	if !ok {
		return nil, fmt.Errorf("no such object %q in container %q", name, container)
	}
	return data, nil
}

// AddContainer creates a new container with the given name, if it
// does not exist. Otherwise an error is returned.
func (s *Swift) AddContainer(name string) error {
	if err := s.ProcessFunctionHook(s, name); err != nil {
		return err
	}
	if s.HasContainer(name) {
		return fmt.Errorf("container already exists %q", name)
	}
	s.mu.Lock()
	s.containers[name] = make(object)
	s.mu.Unlock()
	return nil
}

// ListContainer lists the objects in the given container.
// params contains filtering attributes: prefix, delimiter, marker.
// Only prefix is currently supported.
func (s *Swift) ListContainer(name string, params map[string]string) ([]swift.ContainerContents, error) {
	if err := s.ProcessFunctionHook(s, name); err != nil {
		return nil, err
	}
	if ok := s.HasContainer(name); !ok {
		return nil, fmt.Errorf("no such container %q", name)
	}
	s.mu.Lock()
	items := s.containers[name]
	s.mu.Unlock()
	sorted := make([]string, 0, len(items))
	prefix := params["prefix"]
	for filename := range items {
		if prefix != "" && !strings.HasPrefix(filename, prefix) {
			continue
		}
		sorted = append(sorted, filename)
	}
	sort.Strings(sorted)
	contents := make([]swift.ContainerContents, len(sorted))
	var i = 0
	for _, filename := range sorted {
		contents[i] = swift.ContainerContents{
			Name:         filename,
			Hash:         "", // not implemented
			LengthBytes:  len(items[filename]),
			ContentType:  "application/octet-stream",
			LastModified: time.Now().Format("2006-01-02 15:04:05"), //not implemented
		}
		i++
	}
	return contents, nil
}

// AddObject creates a new object with the given name in the specified
// container, setting the object's data. It's an error if the object
// already exists. If the container does not exist, it will be
// created.
func (s *Swift) AddObject(container, name string, data []byte) error {
	if err := s.ProcessFunctionHook(s, container, name); err != nil {
		return err
	}
	if _, err := s.GetObject(container, name); err == nil {
		return fmt.Errorf(
			"object %q in container %q already exists",
			name,
			container)
	}
	if ok := s.HasContainer(container); !ok {
		if err := s.AddContainer(container); err != nil {
			return err
		}
	}
	s.mu.Lock()
	s.containers[container][name] = data
	s.mu.Unlock()
	return nil
}

// RemoveContainer deletes an existing container with the given name.
func (s *Swift) RemoveContainer(name string) error {
	if err := s.ProcessFunctionHook(s, name); err != nil {
		return err
	}
	if ok := s.HasContainer(name); !ok {
		return fmt.Errorf("no such container %q", name)
	}
	s.mu.Lock()
	delete(s.containers, name)
	s.mu.Unlock()
	return nil
}

// RemoveObject deletes an existing object in a given container.
func (s *Swift) RemoveObject(container, name string) error {
	if err := s.ProcessFunctionHook(s, container, name); err != nil {
		return err
	}
	if _, err := s.GetObject(container, name); err != nil {
		return err
	}
	s.mu.Lock()
	delete(s.containers[container], name)
	s.mu.Unlock()
	return nil
}

// GetURL returns the full URL, which can be used to GET the
// object. An error occurs if the object does not exist.
func (s *Swift) GetURL(container, object string) (string, error) {
	if err := s.ProcessFunctionHook(s, container, object); err != nil {
		return "", err
	}
	if _, err := s.GetObject(container, object); err != nil {
		return "", err
	}
	return s.endpointURL(fmt.Sprintf("%s/%s", container, object)), nil
}
