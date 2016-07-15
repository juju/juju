package envsuite

// Provides an EnvSuite type which makes sure this test suite gets an isolated
// environment settings. Settings will be saved on start and then cleared, and
// reset on tear down.

import (
	"os"
	"strings"

	gc "gopkg.in/check.v1"
)

type EnvSuite struct {
	environ []string
}

func (s *EnvSuite) SetUpSuite(c *gc.C) {
	s.environ = os.Environ()
}

func (s *EnvSuite) SetUpTest(c *gc.C) {
	os.Clearenv()
}

func (s *EnvSuite) TearDownTest(c *gc.C) {
	for _, envstring := range s.environ {
		kv := strings.SplitN(envstring, "=", 2)
		os.Setenv(kv[0], kv[1])
	}
}

func (s *EnvSuite) TearDownSuite(c *gc.C) {
}
