// Copyright 2015 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azuretesting

import (
	"encoding/json"
	"fmt"
	"net/http"
	"regexp"

	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest"
	"github.com/Azure/azure-sdk-for-go/Godeps/_workspace/src/github.com/Azure/go-autorest/autorest/mocks"
)

// MockSender is a wrapper around autorest/mocks.Sender, extending it with
// request path checking to ease testing.
type MockSender struct {
	*mocks.Sender

	// PathPattern, if non-empty, is assumed to be a regular expression
	// that must match the request path.
	PathPattern string
}

func (s *MockSender) Do(req *http.Request) (*http.Response, error) {
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
	sender.EmitContent(string(content))
	return sender
}

// SequentialSender is a Sender that includes a collection of Senders, which
// will be called in sequence.
type Senders []autorest.Sender

func (s *Senders) Do(req *http.Request) (*http.Response, error) {
	if len(*s) == 0 {
		response := mocks.NewResponseWithStatus("", http.StatusInternalServerError)
		return response, fmt.Errorf("no sender for %q", req.URL)
	}
	sender := (*s)[0]
	*s = (*s)[1:]
	return sender.Do(req)
}
