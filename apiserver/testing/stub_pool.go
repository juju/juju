package testing

// StubPoolHelper implements state.PoolHelper
type StubPoolHelper struct {
	StubRelease func() bool
}

func (s StubPoolHelper) Release() bool     { return s.StubRelease() }
func (s StubPoolHelper) Annotate(_ string) {}
