APT_PREREQS=python-dev python3-dev python-virtualenv
PROJECT=mysql
TESTS=tests/

.PHONY: all
all:
	@echo "make clean - Clean all test & build artifacts"
	@echo "make test - Run tests"

.PHONY: clean
clean:
	find . -name '*.pyc' -delete
	rm -rf .venv
	rm -rf .venv3

.venv:
	@echo Processing apt package prereqs
	@for i in $(APT_PREREQS); do dpkg -l | grep -w $$i >/dev/null || sudo apt-get install -y $$i; done
	virtualenv .venv
	.venv/bin/pip install -IUr test_requirements.txt

.venv3:
	@echo Processing apt package prereqs
	@for i in $(APT_PREREQS); do dpkg -l | grep -w $$i >/dev/null || sudo apt-get install -y $$i; done
	virtualenv .venv3 --python=python3
	.venv3/bin/pip install -IUr test_requirements.txt

.PHONY: lint
lint: .venv .venv3
	@echo Checking for Python syntax...
	.venv/bin/flake8 --max-line-length=120 $(PROJECT) $(TESTS) \
	    && echo Py2 OK
	.venv3/bin/flake8 --max-line-length=120 $(PROJECT) $(TESTS) \
	    && echo Py3 OK

# Note we don't even attempt to run tests if lint isn't passing.
.PHONY: test
test: lint test2 test3

.PHONY: test2
test2: .venv
	@echo Starting Py2 tests...
	.venv/bin/nosetests -s --nologcapture tests/

.PHONY: test3
test3: .venv3
	@echo Starting Py3 tests...
	.venv3/bin/nosetests -s --nologcapture tests/
