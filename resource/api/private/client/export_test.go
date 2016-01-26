// Copyright 2016 Canonical Ltd.
// Licensed under the AGPLv3, see LICENCE file for details.

package client

func ExposeClient(client *FacadeClient) (FacadeCaller, UnitDoer) {
	return client.FacadeCaller, client.doer
}
