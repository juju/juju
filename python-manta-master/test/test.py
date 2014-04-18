#!/usr/bin/env python
# Copyright (c) 2012 Joyent, Inc.  All rights reserved.

"""The python-manta test suite entry point."""

import os
from os.path import exists, join, abspath, dirname, normpath
import sys
import logging

import testlib

log = logging.getLogger("test")
testdir_from_ns = {
    None: dirname(__file__),
}

def setup():
    # TODO Perhaps just put this in the test_*.py file.
    lib_dir = dirname(dirname(abspath(__file__)))
    sys.path.insert(0, lib_dir)

if __name__ == "__main__":
    if "TEST_DEBUG" in os.environ:
        level = logging.DEBUG
    else:
        level = logging.INFO
    logging.basicConfig(level=level)

    setup()
    default_tags = []
    retval = testlib.harness(testdir_from_ns=testdir_from_ns,
                             default_tags=default_tags)
    sys.exit(retval)
