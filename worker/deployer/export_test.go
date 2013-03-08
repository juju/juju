package deployer

type fakeAddresser struct{}

func (*fakeAddresser) Addresses() []string {
	return []string{"s1:123", "s2:123"}
}

func NewTestSimpleContext(deployerName, initDir, dataDir, logDir string) *SimpleContext {
	return &SimpleContext{
		addresser:    &fakeAddresser{},
		caCert:       []byte("test-cert"),
		deployerName: deployerName,
		initDir:      initDir,
		dataDir:      dataDir,
		logDir:       logDir,
	}
}
