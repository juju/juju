#!/usr/bin/env python
# coding: utf-8

# Copyright 2014-2015 Canonical Limited.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#  http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from __future__ import print_function

import atexit
import sys

from charmhelpers.contrib.python.rpdb import Rpdb
from charmhelpers.core.hookenv import (
    open_port,
    close_port,
    ERROR,
    log
)

__author__ = "Jorge Niedbalski <jorge.niedbalski@canonical.com>"

DEFAULT_ADDR = "0.0.0.0"
DEFAULT_PORT = 4444


def _error(message):
    log(message, level=ERROR)


def set_trace(addr=DEFAULT_ADDR, port=DEFAULT_PORT):
    """
    Set a trace point using the remote debugger
    """
    atexit.register(close_port, port)
    try:
        log("Starting a remote python debugger session on %s:%s" % (addr,
                                                                    port))
        open_port(port)
        debugger = Rpdb(addr=addr, port=port)
        debugger.set_trace(sys._getframe().f_back)
    except:
        _error("Cannot start a remote debug session on %s:%s" % (addr,
                                                                 port))
