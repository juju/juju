package worker

var LoadedInvalid = make(chan struct{})
func init() {
	loadedInvalid = func() {
		LoadedInvalid <- struct{}{}
	}
}
