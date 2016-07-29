/*
Copyright (c) 2015 VMware, Inc. All Rights Reserved.

Licensed under the Apache License, Version 2.0 (the "License");
you may not use this file except in compliance with the License.
You may obtain a copy of the License at

    http://www.apache.org/licenses/LICENSE-2.0

Unless required by applicable law or agreed to in writing, software
distributed under the License is distributed on an "AS IS" BASIS,
WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
See the License for the specific language governing permissions and
limitations under the License.
*/

package client

import (
	"testing"

	"github.com/juju/govmomi/session"
	"github.com/juju/govmomi/test"
	"github.com/juju/govmomi/vim25"
	"github.com/juju/govmomi/vim25/soap"
	"golang.org/x/net/context"
)

func NewAuthenticatedClient(t *testing.T) *vim25.Client {
	u := test.URL()
	if u == nil {
		t.SkipNow()
	}

	soapClient := soap.NewClient(u, true)
	vimClient, err := vim25.NewClient(context.Background(), soapClient)
	if err != nil {
		t.Fatal(err)
	}

	m := session.NewManager(vimClient)
	err = m.Login(context.Background(), u.User)
	if err != nil {
		t.Fatal(err)
	}

	return vimClient
}
