
default:
	@echo To use this Makefile, please specify which channel you want to build the charm for
	@echo eg `make edge`

.PHONY: edge candidate stable

edge:
	@echo Creating the 'latest/edge' version of juju-qa-test
	cp -r edge/* .
	charmcraft pack
	# TODO: Do we want to rename juju-qa-test_ubuntu_.*.charm to juju-qa-test-edge.charm ?

candidate:
	@echo Creating the 'latest/candidate' version of juju-qa-test
	cp -r candidate/* .
	charmcraft pack

stable:
	@echo Creating the 'latest/stable' version of juju-qa-test
	cp -r stable/* .
	charmcraft pack

20stable:
	@echo Creating the '2.0/stable' version of juju-qa-test
	cp -r 2-0-stable/* .
	charmcraft pack

20edge:
	@echo Creating the '2.0/edge' version of juju-qa-test
	cp -r 2-0-edge/* .
	charmcraft pack

30stable:
	@echo Creating the '3.0/stable' version of juju-qa-test
	cp -r 3-0-stable/* .
	charmcraft pack

