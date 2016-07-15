// Package form enables interactive login without using a web browser.
package form

import (
	"net/url"

	"github.com/juju/httprequest"
	"github.com/juju/loggo"
	"golang.org/x/net/publicsuffix"
	"gopkg.in/errgo.v1"
	"gopkg.in/juju/environschema.v1"
	"gopkg.in/juju/environschema.v1/form"

	"gopkg.in/macaroon-bakery.v1/httpbakery"
)

var logger = loggo.GetLogger("httpbakery.form")

/*
PROTOCOL

A form login works as follows:

	   Client                            Login Service
	      |                                    |
	      | GET visitURL with                  |
	      | "Accept: application/json"         |
	      |----------------------------------->|
	      |                                    |
	      |   Login Methods (including "form") |
	      |<-----------------------------------|
	      |                                    |
	      | GET "form" URL                     |
	      |----------------------------------->|
	      |                                    |
	      |                  Schema definition |
	      |<-----------------------------------|
	      |                                    |
	+-------------+                            |
	|   Client    |                            |
	| Interaction |                            |
	+-------------+                            |
	      |                                    |
	      | POST data to "form" URL            |
	      |----------------------------------->|
	      |                                    |
	      |                Form login response |
	      |<-----------------------------------|
	      |                                    |

The schema is provided as a environschema.Fileds object. It is the
client's responsibility to interpret the schema and present it to the
user.
*/

const (
	// InteractionMethod is the methodURLs key
	// used for a URL that can be used for form-based
	// interaction.
	InteractionMethod = "form"
)

// SchemaRequest is a request for a form schema.
type SchemaRequest struct {
	httprequest.Route `httprequest:"GET"`
}

// SchemaResponse contains the message expected in response to the schema
// request.
type SchemaResponse struct {
	Schema environschema.Fields `json:"schema"`
}

// LoginRequest is a request to perform a login using the provided form.
type LoginRequest struct {
	httprequest.Route `httprequest:"POST"`
	Body              LoginBody `httprequest:",body"`
}

// LoginBody holds the body of a form login request.
type LoginBody struct {
	Form map[string]interface{} `json:"form"`
}

// Visitor implements httpbakery.Visitor
// by providing form-based interaction.
type Visitor struct {
	// Filler holds the form filler that will be used when
	// form-based interaction is required.
	Filler form.Filler
}

// visitWebPage performs the actual visit request. It attempts to
// determine that form login is supported and then download the form
// schema. It calls v.handler.Handle using the downloaded schema and then
// submits the returned form. Any error produced by v.handler.Handle will
// not have it's cause masked.
func (v Visitor) VisitWebPage(client *httpbakery.Client, methodURLs map[string]*url.URL) error {
	return v.visitWebPage(client, methodURLs)
}

// visitWebPage is the internal version of VisitWebPage that operates
// on a Doer rather than an httpbakery.Client, so that we
// can remain compatible with the historic
// signature of the VisitWebPage function.
func (v Visitor) visitWebPage(doer httprequest.Doer, methodURLs map[string]*url.URL) error {
	schemaURL := methodURLs[InteractionMethod]
	if schemaURL == nil {
		return httpbakery.ErrMethodNotSupported
	}
	logger.Infof("got schemaURL %v", schemaURL)
	httpReqClient := &httprequest.Client{
		Doer: doer,
	}
	var s SchemaResponse
	if err := httpReqClient.CallURL(schemaURL.String(), &SchemaRequest{}, &s); err != nil {
		return errgo.Notef(err, "cannot get schema")
	}
	if len(s.Schema) == 0 {
		return errgo.Newf("invalid schema: no fields found")
	}
	host, err := publicsuffix.EffectiveTLDPlusOne(schemaURL.Host)
	if err != nil {
		host = schemaURL.Host
	}
	form, err := v.Filler.Fill(form.Form{
		Title:  "Log in to " + host,
		Fields: s.Schema,
	})
	if err != nil {
		return errgo.NoteMask(err, "cannot handle form", errgo.Any)
	}
	lr := LoginRequest{
		Body: LoginBody{
			Form: form,
		},
	}
	if err := httpReqClient.CallURL(schemaURL.String(), &lr, nil); err != nil {
		return errgo.Notef(err, "cannot submit form")
	}
	return nil
}
