package httpbakery

import (
	"net/http"
	"net/url"

	"github.com/juju/httprequest"
	"gopkg.in/errgo.v1"
)

// ErrMethodNotSupported is the error that a Visitor implementation
// should return if it does not support any of the interaction methods.
var ErrMethodNotSupported = errgo.New("interaction method not supported")

// Visitor represents a handler that can handle ErrInteractionRequired
// errors from a client's discharge request. The methodURLs parameter to
// VisitWebPage holds a set of possible ways to complete the discharge
// request. When called directly from Client, this will contain only a
// single entry with the UserInteractionMethod key, specifying that the
// associated URL should be opened in a web browser for the user to
// interact with.
//
// See FallbackVisitor for a way to gain access to alternative methods.
//
// A Visitor implementation should return ErrMethodNotSupported if it
// cannot handle any of the supplied methods.
type Visitor interface {
	VisitWebPage(client *Client, methodURLs map[string]*url.URL) error
}

const (
	// UserInteractionMethod is the methodURLs key used for a URL
	// that should be visited in a user's web browser. This is also
	// the URL that can be used to fetch the available login methods
	// (with an appropriate Accept header).
	UserInteractionMethod = "interactive"
)

type multiVisitor struct {
	supportedMethods []Visitor
}

// NewMultiVisitor returns a Visitor that queries the discharger for
// available methods and then tries each of the given visitors in turn
// until one succeeds or fails with an error cause other than
// ErrMethodNotSupported.
func NewMultiVisitor(methods ...Visitor) Visitor {
	return &multiVisitor{
		supportedMethods: methods,
	}
}

// VisitWebPage implements Visitor.VisitWebPage by obtaining all the
// available interaction methods and calling v.supportedMethods until it
// finds one that recognizes the method. If a Visitor returns an error
// other than ErrMethodNotSupported the error will be immediately
// returned to the caller; its cause will not be masked.
func (v multiVisitor) VisitWebPage(client *Client, methodURLs map[string]*url.URL) error {
	// The Client implementation will always include a UserInteractionMethod
	// entry taken from the VisitURL field in the error, so use that
	// to find the set of supported interaction methods.
	u := methodURLs[UserInteractionMethod]
	if u == nil {
		return errgo.Newf("cannot get interaction methods because no %q URL found", UserInteractionMethod)
	}
	if urls, err := GetInteractionMethods(client, u); err == nil {
		// We succeeded in getting the set of interaction methods from
		// the discharger. Use them.
		methodURLs = urls
		if methodURLs[UserInteractionMethod] == nil {
			// There's no "interactive" method returned, but we know
			// the server does actually support it, because all dischargers
			// are required to, so fill it in with the original URL.
			methodURLs[UserInteractionMethod] = u
		}
	} else {
		logger.Debugf("ignoring error: cannot get interaction methods: %v", err)
	}
	// Go through all the Visitors, looking for one that supports one
	// of the methods we have found.
	for _, m := range v.supportedMethods {
		err := m.VisitWebPage(client, methodURLs)
		if err == nil {
			return nil
		}
		if errgo.Cause(err) != ErrMethodNotSupported {
			return errgo.Mask(err, errgo.Any)
		}
	}
	return errgo.Newf("no methods supported")
}

// GetInteractionMethods queries a URL as found in an
// ErrInteractionRequired VisitURL field to find available interaction
// methods.
//
// It does this by sending a GET request to the URL with the Accept
// header set to "application/json" and parsing the resulting
// response as a map[string]string.
//
// It uses the given Doer to execute the HTTP GET request.
func GetInteractionMethods(client httprequest.Doer, u *url.URL) (map[string]*url.URL, error) {
	httpReqClient := &httprequest.Client{
		Doer: client,
	}
	req, err := http.NewRequest("GET", u.String(), nil)
	if err != nil {
		return nil, errgo.Notef(err, "cannot create request")
	}
	req.Header.Set("Accept", "application/json")
	var methodURLStrs map[string]string
	if err := httpReqClient.Do(req, nil, &methodURLStrs); err != nil {
		return nil, errgo.Mask(err)
	}
	// Make all the URLs relative to the request URL.
	methodURLs := make(map[string]*url.URL)
	for m, urlStr := range methodURLStrs {
		relURL, err := url.Parse(urlStr)
		if err != nil {
			return nil, errgo.Notef(err, "invalid URL for interaction method %q", m)
		}
		methodURLs[m] = u.ResolveReference(relURL)
	}
	return methodURLs, nil
}
