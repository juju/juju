package api

// Add methods that do not interact with the state
// server so we can benchmark request time
// independent of mongo.
type rpcId struct {
	Id string
}

func (st *State) TestRequest() error {
	var resp rpcId
	err := st.client.Call("Testing", "", "Request", nil, &resp)
	if err != nil {
		return rpcError(err)
	}
	if resp.Id != "reply" {
		panic("unexpected response")
	}
	return nil
}

func (st *srvState) Testing(id string) (*srvTesting, error) {
	return &srvTesting{}, nil
}

type srvTesting struct{}

func (t *srvTesting) Request() (rpcId, error) {
	return rpcId{"reply"}, nil
}
