p=test*.py
test:
	TMPDIR=/tmp python -m unittest discover -vv ./tests -p "$(p)"
lint:
	flake8 $$(find -name '*.py')
cover:
	python -m coverage run --source="./" --omit "./tests/*" -m unittest discover -vv ./tests
	python -m coverage report
clean:
	find . -name '*.pyc' -delete
apt-update:
	sudo apt-get -qq update
juju-ci-tools.common_0.1.0-0_all.deb: apt-update
	sudo apt-get install -y equivs
	equivs-build juju-ci-tools-common
install-deps: juju-ci-tools.common_0.1.0-0_all.deb apt-update
	sudo dpkg -i juju-ci-tools.common_0.1.0-0_all.deb || true
	sudo apt-get install -y -f
	sudo apt-get install -y juju-local juju juju-quickstart juju-deployer
	sudo apt-get install -y python-pip
	./pipdeps.py install
.PHONY: lint test apt-update install-deps
