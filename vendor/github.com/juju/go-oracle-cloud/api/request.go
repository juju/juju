// Copyright 2017 Canonical Ltd.
// Licensed under the LGPLv3, see LICENCE file for details.

package api

import (
	"bytes"
	"encoding/json"
	"io"
	"net/http"
)

// endoiints map for every resource in the oracle iaas api:w
var endpoints = map[string]string{
	"account":               "%s/account",
	"acl":                   "%s/network/v1/acl",
	"authenticate":          "%s/authenticate/",
	"backupconfiguration":   "%s/backupservice/v1/configuration",
	"backup":                "%s/backupservice/v1/backup",
	"imagelistentrie":       "%s/imagelist",
	"imagelist":             "%s/imagelist",
	"instanceconsole":       "%s/instanceconsole",
	"instance":              "%s/instance",
	"ipaddressassociation":  "%s/network/v1/ipassociation",
	"ipaddressprefixset":    "%s/network/v1/ipaddressprefixset",
	"ipaddressreservation":  "%s/network/v1/ipreservation",
	"ipassociation":         "%s/ip/association",
	"ipnetworkexchange":     "%s/network/v1/ipnetworkexchange",
	"ipnetwork":             "%s/network/v1/ipnetwork",
	"ipreservation":         "%s/ip/reservation",
	"launchplan":            "%s/launchplan/",
	"machineimage":          "%s/machineimage",
	"orchestration":         "%s/orchestration",
	"osscontainer":          "%s/integrations/osscontainer",
	"rebootinstancerequest": "%s/rebootinstancerequest",
	"refreshtoken":          "%s/refresh",
	"restore":               "%s/backupservice/v1/restore",
	"route":                 "%s/network/v1/route",
	"secapplication":        "%s/secapplication",
	"secassociation":        "%s/secassociation",
	"seciplist":             "%s/seciplist",
	"seclist":               "%s/seclist",
	"secrule":               "%s/secrule",
	"securityprotocol":      "%s/network/v1/secprotocol",
	"securityrule":          "%s/network/v1/secrule",
	"shape":                 "%s/shape",
	"snapshot":              "%s/snapshot",
	"sshkey":                "%s/sshkey",
	"storageattachment":     "%s/storage/attachment",
	"storageproperty":       "%s/property/storage",
	"storagesnapshot":       "%s/storage/snapshot",
	"storagevolume":         "%s/storage/volume",
	"virtualnic":            "%s/network/v1/vnic",
	"virtualnicset":         "%s/network/v1/vnicset",
	"vpnendpoint":           "%s/vpnendpoint",
}

// treatStatus will be used as a callback to custom check the response
// if the client decides the response contains some error codes
// it will make the Request return that error
type treatStatus func(resp *http.Response, verbRequest string) error

// defaultTreat used in post, put, delete and get requests
func defaultTreat(resp *http.Response, verbRequest string) (err error) {
	switch resp.StatusCode {
	// this is the case when if we have such status
	// we return nil
	case http.StatusOK, http.StatusCreated, http.StatusNoContent:
		return nil
	case http.StatusBadRequest:
		return errBadRequest.DumpApiError(resp.Body)
	case http.StatusUnauthorized:
		return errNotAuthorized.DumpApiError(resp.Body)
	case http.StatusPaymentRequired:
		return errPaymentRequired.DumpApiError(resp.Body)
	case http.StatusForbidden:
		return errForbidden.DumpApiError(resp.Body)
	case http.StatusMethodNotAllowed:
		return errMethodNotAllowed.DumpApiError(resp.Body)
	case http.StatusNotAcceptable:
		return errNotAcceptable.DumpApiError(resp.Body)
	case http.StatusRequestTimeout:
		return errRequestTimeout.DumpApiError(resp.Body)
	case http.StatusGone:
		return errGone.DumpApiError(resp.Body)
	case http.StatusLengthRequired:
		return errLengthRequired.DumpApiError(resp.Body)
	case http.StatusRequestEntityTooLarge:
		return errRequestEntityTooLarge.DumpApiError(resp.Body)
	case http.StatusPreconditionRequired:
		return errPreconditionRequired.DumpApiError(resp.Body)
	case http.StatusRequestURITooLong:
		return errURITooLong.DumpApiError(resp.Body)
	case http.StatusUnsupportedMediaType:
		return errUnsupportedMediaType.DumpApiError(resp.Body)
	case http.StatusRequestedRangeNotSatisfiable:
		return errRequestNotSatisfiable.DumpApiError(resp.Body)
	case http.StatusInternalServerError:
		return errInternalApi.DumpApiError(resp.Body)
	case http.StatusNotImplemented:
		return errNotImplemented.DumpApiError(resp.Body)
	case http.StatusServiceUnavailable:
		return errServiceUnavailable.DumpApiError(resp.Body)
	case http.StatusGatewayTimeout:
		return errGatewayTimeout.DumpApiError(resp.Body)
	case http.StatusHTTPVersionNotSupported:
		return errNotSupported.DumpApiError(resp.Body)
	case http.StatusConflict:
		return errStatusConflict.DumpApiError(resp.Body)
	case http.StatusNotFound:
		if verbRequest == http.MethodDelete {
			return nil
		}
		return errNotFound.DumpApiError(resp.Body)
	// any other error
	default:
		return dumpApiError(resp)
	}
}

// Filter type that holds A Arg-Value, this should be present
// as argument in all functions that start with the prefix All
type Filter struct {
	// Arg is the key of the Key-Value pair
	Arg string
	// Value is the value of the Key-Value pair
	Value string
}

// paramsRequest used to fill up the params for the request function
// this will hold different configurations such as the url of the request
// directory flag, treat function, filters, etc.
type paramsRequest struct {
	// directory is the type of directory request
	// this will make the request have a special
	// Header request appended in order to make the api
	// list names,containers
	directory bool
	// url this will not be checked so, make sure it is valid
	url string
	// verb contains an http verb like for example POST,PUT,GET,DELETE
	verb string
	// body will contain the http body request if any
	// for Get request this should be leaved nil
	body interface{}
	// treat will be used as a callback to
	// check the response and save extract things
	treat treatStatus
	// resp will contains a json object where the
	// request function will decode
	resp interface{}
	// filter is the query args for All* type functions
	filter []Filter

	// In case of authentication error, we need to refresh
	// the token and retry the request. This allows us to track
	// whether or not this is a retry or the initial request
	isRetry bool
}

// request function is a wrapper around building the request,
// treating exceptional errors and executing the client http connection
func (c *Client) request(cfg paramsRequest) (err error) {
	var buf io.Reader

	// if we have a body we assume that the body
	// should be json encoded and ready to
	// be appendend into the request
	if cfg.body != nil {
		raw, err := json.Marshal(cfg.body)
		if err != nil {
			return err
		}
		buf = bytes.NewBuffer(raw)
	}

	req, err := http.NewRequest(cfg.verb, cfg.url, buf)
	if err != nil {
		return err
	}

	// if the filter is specified in the config
	// this means we want the query to be filtered
	if cfg.filter != nil && len(cfg.filter) > 0 {
		q := req.URL.Query()
		for _, val := range cfg.filter {
			// add the key-value query pair
			q.Add(val.Arg, val.Value)
		}
		// after we've added all filters
		// we now encode them into the base url
		req.URL.RawQuery = q.Encode()
	}
	// add the session cookie if there is one
	if c.cookie != nil {
		req.AddCookie(c.cookie)
	}

	// let the endpoint api know that the request
	// was made from the go oracle cloud client
	req.Header.Add("User-Agent", "go-oracle-cloud v1.0")
	// the oracle api supports listing so in order to use it
	// we should include the +directory header
	if cfg.directory {
		req.Header.Add("Accept", "application/oracle-compute-v3+directory+json")
	} else {
		req.Header.Add("Accept", "application/oracle-compute-v3+json")
	}
	// every request should let know that we accept encoded gzip responses
	req.Header.Add("Accept-Encoding", "gzip;q=1.0, identity; q=0.5")

	switch cfg.verb {
	case "POST", "PUT":
		req.Header.Add("Content-Encoding", "deflate")
		req.Header.Add("Content-Type", "application/oracle-compute-v3+json")
	case "DELETE":
		req.Header.Add("Content-Type", "application/oracle-compute-v3+json")
	case "GET":
	}

	resp, err := c.http.Do(req)
	if err != nil {
		return err
	}

	defer func() {
		if errClose := resp.Body.Close(); errClose != nil {
			// overwrite the previous error if any
			err = errClose
		}
	}()

	var errTreat error
	// if we have a special treat function
	// let the caller treat the response
	// if we don't have one execute the default one
	if cfg.treat != nil {
		errTreat = cfg.treat(resp, cfg.verb)
	} else {
		errTreat = defaultTreat(resp, cfg.verb)
	}

	// if the status code of the request is unauthorized
	// this means the cookie has expired and we need to
	// refresh it.
	if IsNotAuthorized(errTreat) && c.isAuth() {
		if err := c.RefreshCookie(); err != nil {
			return err
		}
		if !cfg.isRetry {
			// resend the request after the refresh
			newCfg := cfg
			newCfg.isRetry = true
			return c.request(newCfg)
		}
	} else if errTreat != nil {
		return errTreat
	}

	// if we the caller tells us that the http request
	// returns an response and wants to decode the response
	// that is json format.
	if cfg.resp != nil {
		if err = json.NewDecoder(resp.Body).Decode(cfg.resp); err != nil {
			return err
		}
	}

	return nil
}
