mkfile_dir          := $(dir $(abspath $(lastword $(MAKEFILE_LIST))))
p                   = test*.py
py3                 = assess_model_change_watcher.py
python_base_path    = $(shell which python3)
python_path         = $(mkfile_dir)venv
python_bin_path     = $(python_path)/bin

.PHONY: test
test: ensure-venv
	TMPDIR=/tmp $(python_bin_path)/python3 -m unittest discover -vv . -p "$(p)"
	TMPDIR=/tmp $(python_bin_path)/python3 -m unittest discover -vv jujupy -t . -p "$(p)"

.PHONY: lint
lint: ensure-venv
	$(python_bin_path)/python3 -m flake8 --builtins xrange,basestring $(py3) --exclude=repository,venv
	$(python_bin_path)/flake8 --builtins xrange,basestring --exclude=$(py3),repository,venv

.PHONY: cover
cover: ensure-venv
	python -m coverage run --source="./" --omit "./tests/*" -m unittest discover -vv ./tests
	python -m coverage report

.PHONY: clean
clean:
	find . -name '*.pyc' -delete

.PHONY: apt-update
apt-update:
	sudo apt-get -qq update

juju-ci-tools.common_0.1.4-0_all.deb: apt-update
	find . -name '*.deb' -delete
	sudo apt-get install -y equivs
	equivs-build juju-ci-tools-common

.PHONY: install-deps
install-deps: juju-ci-tools.common_0.1.4-0_all.deb apt-update
	sudo dpkg -i juju-ci-tools.common_0.1.4-0_all.deb || true
	sudo apt-get install -y -f
	$(python_bin_path)/pip3 install -r $(mkfile_dir)requirements-dev.txt

name        = NAMEHERE
assess_file = assess_$(name).py

.PHONY: new-assess
new-assess: ensure-venv
	install -m 755 template_assess.py.tmpl $(assess_file)
	sed -i -e "s/TEMPLATE/$(name)/g" $(assess_file) $(test_assess_file)

.PHONY: ensure-venv
ensure-venv:
	test -d venv || virtualenv -p $(python_base_path) $(python_path)
	. $(python_bin_path)/activate
