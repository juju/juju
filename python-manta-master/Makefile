# Copyright (c) 2012, Joyent, Inc. All rights reserved.
#
# Makefile for python-manta
#

TOP := $(shell pwd)
NAME		:= python-manta

ifeq ($(shell uname -s),Darwin)
	# http://superuser.com/questions/61185
	# http://forums.macosxhints.com/archive/index.php/t-43243.html
	# This is an Apple customization to `tar` to avoid creating
	# '._foo' files for extended-attributes for archived files.
	TAR := COPYFILE_DISABLE=true COPY_EXTENDED_ATTRIBUTES_DISABLE=true tar
else
	TAR := tar
endif


#
# Targets
#
.PHONY: all
all:

.PHONY: clean
clean:
	find lib test -name "*.pyc" | xargs rm
	find lib test -name "*.pyo" | xargs rm
	find lib test -name "__pycache__" | xargs rm -rf
	rm -rf dist
	rm -rf manta.egg-info

.PHONY: test
test:
	python test/test.py $(TAGS)
.PHONY: test-kvm6
test-kvm6:
	make test MANTA_URL=https://10.2.126.200 MANTA_INSECURE=1 MANTA_USER=trent

.PHONY: testall
testall:
	python test/testall.py

.PHONY: cutarelease
cutarelease:
	./tools/cutarelease.py -f manta/version.py

# Only have this around to retry package uploads on a tag created by
# 'make cutarelease' because PyPI upload is super-flaky (at least for me).
.PHONY: pypi-upload
pypi-upload:
	COPY_EXTENDED_ATTRIBUTES_DISABLE=1 python setup.py sdist --formats zip upload
