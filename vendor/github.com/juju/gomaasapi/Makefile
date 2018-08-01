# Build, and run tests.
check: examples
	go test ./...

example_source := $(wildcard example/*.go)
example_binaries := $(patsubst %.go,%,$(example_source))

# Clean up binaries.
clean:
	$(RM) $(example_binaries)

# Reformat the source files to match our layout standards.
format:
	gofmt -w .

# Invoke gofmt's "simplify" option to streamline the source code.
simplify:
	gofmt -w -s .

# Build the examples (we have no tests for them).
examples: $(example_binaries)

%: %.go
	go build -o $@ $<

.PHONY: check clean format examples simplify
