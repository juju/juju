(create-a-unit-test-suite)=
# Create a unit test suite
> See also: [Unit test suite](/doc/dev/reference/testing/unit-testing/unit-test-suite.md)

To create a new unit test suite, you can do something like:

```go

type ToolsSuite struct {

testing.BaseSuite

dataDir string

}

func TestToolsSuite(t *stdtesting.T) {
tc.Run(t, &ToolsSuite{})
}

```

If there is no extra setup needed, then you don't need to specify any set-up or tear-down methods as the LoggingSuite
has them, and they are called by default.

If you did want to do something, say, create a directory and save it in the dataDir, you would do something like this:

```go

func (t *ToolsSuite) SetUpTest(c *tc.C) {

t.BaseSuite.SetUpTest(c)

t.dataDir = c.MkDir()

}

```

If the test suite has multiple contained suites, please call them in the order that they are defined, and make sure
something that is composed from the BaseSuite is first. They should be torn down in the reverse order.

Even if the code that is being tested currently has no logging or outbound network access in it, it is a good idea to
use the BaseSuite as a base:

* it isolates the user's home directory against accidental modification
* if someone does add outbound network access later, it will be caught
* it brings in something composed of the CleanupSuite
* if someone does add logging later, it is captured and doesn't pollute the logging output
