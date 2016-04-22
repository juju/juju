p=test*.py
test:
	TMPDIR=/tmp python -m unittest discover -vv ./tests -p "$(p)"
lint:
	flake8 $$(find -name '*.py') --builtins xrange,basestring
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
name=NAMEHERE
assess_file=assess_$(name).py
test_assess_file=tests/test_assess_$(name).py
new-assess:
	install -m 755 template_assess.py.tmpl $(assess_file)
	install -m 644 template_test.py.tmpl $(test_assess_file)
	sed -i -e "s/TEMPLATE/$(name)/g" $(assess_file) $(test_assess_file)
.PHONY: lint test cover clean new-assess apt-update install-deps
