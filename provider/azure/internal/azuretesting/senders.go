// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"
	"strings"
	"sync"

	"github.com/Azure/go-autorest/autorest"
	"github.com/Azure/go-autorest/autorest/mocks"
	"github.com/juju/errors"
	"github.com/juju/loggo"

	"github.com/juju/juju/version"
)

var logger = loggo.GetLogger("juju.provider.azure.internal.azuretesting")

// MockSender is a wrapper around autorest/mocks.Sender, extending it with
// request path checking to ease testing.
type MockSender struct {
	*mocks.Sender

	// PathPattern, if non-empty, is assumed to be a regular expression
	// that must match the request path.
	PathPattern string
}

func (s *MockSender) Do(req *http.Request) (*http.Response, error) {
	if ua := req.UserAgent(); !strings.HasPrefix(ua, "Juju/"+version.Current.String()) {
		return nil, errors.Errorf("request has unexpected User-Agent %q", ua)
	}
	if s.PathPattern != "" {
		matched, err := regexp.MatchString(s.PathPattern, req.URL.Path)
		if err != nil {
			return nil, err
		}
		if !matched {
			return nil, fmt.Errorf(
				"request path %q did not match pattern %q",
				req.URL.Path, s.PathPattern,
			)
		}
	}
	return s.Sender.Do(req)
}

// NewSenderWithValue returns a *mocks.Sender that marshals the provided object
// to JSON and sets it as the content. This function will panic if marshalling
// fails.
func NewSenderWithValue(v interface{}) *MockSender {
	content, err := json.Marshal(v)
	if err != nil {
		panic(err)
	}
	sender := &MockSender{Sender: mocks.NewSender()}
	sender.AppendResponse(mocks.NewResponseWithContent(string(content)))
	return sender
}

// Senders is a Sender that includes a collection of Senders, which
// will be called in sequence.
type Senders []autorest.Sender

func (s *Senders) Do(req *http.Request) (*http.Response, error) {
	logger.Debugf("Senders.Do(%s)", req.URL)
	if len(*s) == 0 {
		response := mocks.NewResponseWithStatus("", http.StatusInternalServerError)
		return response, fmt.Errorf("no sender for %q", req.URL)
	}
	sender := (*s)[0]
	*s = (*s)[1:]
	return sender.Do(req)
}

// SerialSender is a Sender that permits only one active Do call
// at a time.
type SerialSender struct {
	mu sync.Mutex
	s  autorest.Sender
}

func (s *SerialSender) Do(req *http.Request) (*http.Response, error) {
	s.mu.Lock()
	defer s.mu.Unlock()
	return s.s.Do(req)
}

func NewSerialSender(s autorest.Sender) *SerialSender {
	return &SerialSender{s: s}
}
