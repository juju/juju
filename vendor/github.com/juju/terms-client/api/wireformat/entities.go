// Copyright 2016 Canonical Ltd.
// Licensed under the GPLv3, see LICENCE file for details.

// The wireformat package contains definitions of wireformat used by the terms
// service.
package wireformat

import (
	"fmt"
	"time"

	"github.com/juju/errors"
)

// SaveTerm structure contains the content of the terms document
// to be saved.
type SaveTerm struct {
	Content string `json:"content"`
}

// Validate validates the save term request.
func (t *SaveTerm) Validate() error {
	if t.Content == "" {
		return errors.BadRequestf("empty term content")
	}
	return nil
}

// Term contains the terms and conditions document structure.
type Term struct {
	Owner     string      `json:"owner,omitempty" yaml:"owner,omitempty"`
	Name      string      `json:"name" yaml:"name"`
	Revision  int         `json:"revision" yaml:"revision"`
	Title     string      `json:"title,omitempty" yaml:"title,omitempty"`
	CreatedOn TimeRFC3339 `json:"created-on,omitempty" yaml:"createdon"`
	Published bool        `json:"published", yaml:"published"`
	Content   string      `json:"content,omitempty" yaml:"content,omitempty"`
}

// TermIDResponse contains just the termID
type TermIDResponse struct {
	TermID string `json:"term-id"`
}

func (t *Term) termID() string {
	return fmt.Sprintf("%s/%s/%d", t.Owner, t.Name, t.Revision)
}

// Terms stores a sortable slice of terms.
type Terms []Term

// Len implements sort.Interface
func (terms Terms) Len() int {
	return len([]Term(terms))
}

// Less implements sort.Interface
func (terms Terms) Less(i, j int) bool {
	return terms[i].termID() < terms[j].termID()
}

// Swap implements sort.Interface
func (terms Terms) Swap(i, j int) {
	terms[i], terms[j] = terms[j], terms[i]
}

// AgreementRequest holds the parameters for creating a new
// user agreement to a specific revision of terms.
type AgreementRequest struct {
	TermOwner    string `json:"termowner"`
	TermName     string `json:"termname"`
	TermRevision int    `json:"termrevision"`
}

// Agreements holds multiple agreements peformed in
// a single request.
type Agreements struct {
	Agreements []Agreement `json:"agreements"`
}

// Agreement holds a single agreement made by
// the user to a specific revision of terms and conditions
// document.
type Agreement struct {
	User      string      `json:"user"`
	Owner     string      `json:"owner"`
	Term      string      `json:"term"`
	Revision  int         `json:"revision"`
	CreatedOn TimeRFC3339 `json:"created-on"`
}

// TimeRFC3339 represents a time, which is marshaled
// and unmarshaled using the RFC3339 format
type TimeRFC3339 time.Time

// MarshalJSON implements the json.Marshaler interface.
func (t TimeRFC3339) MarshalJSON() ([]byte, error) {
	b := make([]byte, 0, len(time.RFC3339)+2)
	b = append(b, '"')
	b = time.Time(t).AppendFormat(b, time.RFC3339)
	b = append(b, '"')
	return b, nil
}

// MarshalYAML implements gopkg.in/juju/yaml.v2 Marshaler interface.
func (t TimeRFC3339) MarshalYAML() (interface{}, error) {
	return time.Time(t).Format(time.RFC3339), nil
}

// UnmarshalJSON implements the json.Unmarshaler interface.
func (t *TimeRFC3339) UnmarshalJSON(data []byte) error {
	t0, err := time.Parse(`"`+time.RFC3339+`"`, string(data))
	if err != nil {
		return errors.Trace(err)
	}
	*t = TimeRFC3339(t0)
	return nil
}

// UnmarshalYAML implements the yaml.Unmarshaler interface.
func (t *TimeRFC3339) UnmarshalYAML(unmarshal func(interface{}) error) error {
	var data string
	err := unmarshal(&data)
	if err != nil {
		return errors.Trace(err)
	}
	t0, err := time.Parse(time.RFC3339, data)
	if err != nil {
		return errors.Trace(err)
	}
	*t = TimeRFC3339(t0)
	return nil
}

// DebugStatusResponse contains results of various checks that
// form the status of the terms service.
type DebugStatusResponse struct {
	Checks map[string]CheckResult `json:"checks"`
}

// CheckResult holds the result of a single status check.
type CheckResult struct {
	// Name is the human readable name for the check.
	Name string `json:"name"`

	// Value is the check result.
	Value string `json:"value"`

	// Passed reports whether the check passed.
	Passed bool `json:"passed"`

	// Duration holds the duration that the
	// status check took to run.
	Duration time.Duration `json:"duration"`
}

// GetTermsResponse holds the response of the GetTerms call.
type GetTermsResponse struct {
	Name      string    `json:"name" yaml:"name"`
	Owner     string    `json:"owner,omitempty" yaml:"owner,omitempty"`
	Title     string    `json:"title" yaml:"title"`
	Revision  int       `json:"revision" yaml:"revision"`
	CreatedOn time.Time `json:"created-on" yaml:"createdon"`
	Content   string    `json:"content" yaml:"content"`
}

// CheckAgreementsRequest holds a slice of terms and the /v1/agreement
// endpoint will check if the user has agreed to the specified terms
// and return a slice of terms the user has not agreed to yet.
type CheckAgreementsRequest struct {
	Terms []string
}

// SaveAgreements holds the parameters for creating new
// user agreements to one or more specific revisions of terms.
type SaveAgreements struct {
	Agreements []SaveAgreement `json:"agreements"`
}

// SaveAgreement holds the parameters for creating a new
// user agreement to a specific revision of terms.
type SaveAgreement struct {
	TermOwner    string `json:"termowner"`
	TermName     string `json:"termname"`
	TermRevision int    `json:"termrevision"`
}

// SaveAgreementResponses holds the response of the SaveAgreement
// call.
type SaveAgreementResponses struct {
	Agreements []AgreementResponse `json:"agreements"`
}

// AgreementResponse holds the a single agreement made by
// the user to a specific revision of terms and conditions
// document.
type AgreementResponse struct {
	User      string    `json:"user" yaml:"user"`
	Owner     string    `json:"owner,omitempty" yaml:"owner,omitempty"`
	Term      string    `json:"term" yaml:"term"`
	Revision  int       `json:"revision" yaml:"revision"`
	CreatedOn time.Time `json:"created-on" yaml:"createdon"`
}
