test:
	python -m unittest discover -vv . -p '*.py'
lint:
	flake8 .
apt-update:
	sudo apt-get update
DEPS_VERSION := 0.1.1-0
juju-ci-tools.common_$(DEPS_VERSION)_all.deb: apt-update
	sudo apt-get install -y equivs
	equivs-build juju-ci-tools-common
install-deps: juju-ci-tools.common_$(DEPS_VERSION)_all.deb apt-update
	sudo dpkg -i juju-ci-tools.common_$(DEPS_VERSION)_all.deb || true
	sudo apt-get install -y -f
.PHONY: lint test apt-update install-deps
