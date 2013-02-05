package api

// Add methods that do not interact with the state
// server so we can benchmark request time
// independent of mongo.

type testResp struct {
	X string
}

func (st *State) TestRequest() error {
	var resp testResp
	err := st.client.Call("Testing", "", "Request", nil, &resp)
	if err != nil {
		return rpcError(err)
	}
	if resp.X != "reply" {
		panic("unexpected response")
	}
	return nil
}

func (st *srvRoot) Testing(id string) (*srvTesting, error) {
	return &srvTesting{}, nil
}

type srvTesting struct{}

func (t *srvTesting) Request() (testResp, error) {
	return testResp{"reply"}, nil
}
