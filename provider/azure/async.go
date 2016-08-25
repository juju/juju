// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package azure

import (
	"bytes"
	"encoding/json"
	"io/ioutil"
	"net/http"

	"github.com/Azure/go-autorest/autorest"
)

// asyncCreationRespondDecorator returns an autorest.RespondDecorator
// that replaces non-failure provisioning states with "Succeeded", to
// prevent the autorest code from blocking until the resource is completely
// provisioned.
func asyncCreationRespondDecorator(original autorest.RespondDecorator) autorest.RespondDecorator {
	return func(r autorest.Responder) autorest.Responder {
		return autorest.ResponderFunc(func(resp *http.Response) error {
			if resp.Body != nil {
				if err := overrideProvisioningState(resp); err != nil {
					return err
				}
			}
			return original(r).Respond(resp)
		})
	}
}

func overrideProvisioningState(resp *http.Response) error {
	var buf bytes.Buffer
	if _, err := buf.ReadFrom(resp.Body); err != nil {
		return err
	}
	if err := resp.Body.Close(); err != nil {
		return err
	}
	resp.Body = ioutil.NopCloser(&buf)

	body := make(map[string]interface{})
	if err := json.Unmarshal(buf.Bytes(), &body); err != nil {
		// Don't treat failure to decode the body as an error,
		// or we may get in the way of response handling.
		return nil
	}
	properties, ok := body["properties"].(map[string]interface{})
	if !ok {
		// No properties, nothing to do.
		return nil
	}
	provisioningState, ok := properties["provisioningState"]
	if !ok {
		// No provisioningState, nothing to do.
		return nil
	}

	switch provisioningState {
	case "Canceled", "Failed", "Succeeded":
		// In any of these cases, pass on the body untouched.
	default:
		properties["provisioningState"] = "Succeeded"
		buf.Reset()
		if err := json.NewEncoder(&buf).Encode(body); err != nil {
			return err
		}
	}
	return nil
}
