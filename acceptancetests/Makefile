mkfile_dir := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
p=test*.py
py3="assess_model_change_watcher.py"
flake8_ignore="W503,E121,E722,E123,E126"

define TEST_DEPENDENCIES
	libssl-dev
	python-pip
	python3-pip
	python-tz
	python-yaml
endef

.PHONY: install-test-dependencies
install-test-dependencies: apt-update
	@echo Installing test dependecies
	@sudo apt-get --yes install \
	$(strip $(TEST_DEPENDENCIES))
	pip install --user -r $(mkfile_dir)/requirements.txt
	pip3 install --user -r $(mkfile_dir)/requirements.txt

# "-W ignore::DeprecationWarning" should be removed once moved to py3 only.
test:
	TMPDIR=/tmp python -m unittest discover . -p "$(p)"
	TMPDIR=/tmp python3 -W ignore::DeprecationWarning -m unittest discover jujupy -t . -p "$(p)"
lint:
	python3 -m flake8 --ignore=$(flake8_ignore) --builtins xrange $(py3) --exclude=repository $(mkfile_dir)
	flake8 --ignore=$(flake8_ignore) --builtins xrange --exclude=$(py3),repository $(mkfile_dir)
cover:
	python -m coverage run --source="./" --omit "./tests/*" -m unittest discover -vv ./tests
	python -m coverage report
clean:
	find . -name '*.pyc' -delete

apt-update:
	sudo apt-get -qq update
juju-ci-tools.common_0.1.4-0_all.deb: apt-update
	find . -name '*.deb' -delete
	sudo apt-get install -y equivs
	equivs-build juju-ci-tools-common
install-deps: juju-ci-tools.common_0.1.4-0_all.deb apt-update
	sudo dpkg -i juju-ci-tools.common_0.1.4-0_all.deb || true
	sudo apt-get install -y -f
	pip install --user -r $(mkfile_dir)/requirements.txt
name=NAMEHERE
assess_file=assess_$(name).py
new-assess:
	install -m 755 template_assess.py.tmpl $(assess_file)
	sed -i -e "s/TEMPLATE/$(name)/g" $(assess_file) $(test_assess_file)
.PHONY: lint test cover clean new-assess apt-update install-deps
