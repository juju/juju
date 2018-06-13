.PHONY: all
all:
	@echo "make clean - Clean all test & doc build artifacts"
	@echo "make lint  - Run linter"
	@echo "make docs  - Build html documentation"

.PHONY: clean
clean:
	git clean -fxd

.PHONY: lint
lint:
	tox -e lint

.PHONY: tox
tox:
	tox

.PHONY: docs
docs:
	tox -e docs
