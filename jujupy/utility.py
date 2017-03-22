# Copyright 2014-2017 Canonical Ltd.
#
# Licensed under the Apache License, Version 2.0 (the "License");
# you may not use this file except in compliance with the License.
# You may obtain a copy of the License at
#
#     http://www.apache.org/licenses/LICENSE-2.0
#
# Unless required by applicable law or agreed to in writing, software
# distributed under the License is distributed on an "AS IS" BASIS,
# WITHOUT WARRANTIES OR CONDITIONS OF ANY KIND, either express or implied.
# See the License for the specific language governing permissions and
# limitations under the License.

from contextlib import contextmanager
from datetime import (
    datetime,
    )
import errno
import os
import re
from shutil import rmtree
import subprocess
import socket
import sys
from time import (
    sleep,
    )
from tempfile import (
    mkdtemp,
    NamedTemporaryFile,
    )
# Export shell quoting function which has moved in newer python versions
try:
    from shlex import quote
except ImportError:
    from pipes import quote
import yaml

quote


@contextmanager
def scoped_environ(new_environ=None):
    """Save the current environment and restore it when the context is exited.

    :param new_environ: If provided and not None, the key/value pairs of the
    iterable are used to create a new environment in the context."""
    old_environ = dict(os.environ)
    try:
        if new_environ is not None:
            os.environ.clear()
            os.environ.update(new_environ)
        yield
    finally:
        os.environ.clear()
        os.environ.update(old_environ)


class until_timeout:

    """Yields remaining number of seconds.  Stops when timeout is reached.

    :ivar timeout: Number of seconds to wait.
    """
    def __init__(self, timeout, start=None):
        self.timeout = timeout
        if start is None:
            start = self.now()
        self.start = start

    def __iter__(self):
        return self

    @staticmethod
    def now():
        return datetime.now()

    def __next__(self):
        return self.next()

    def next(self):
        elapsed = self.now() - self.start
        remaining = self.timeout - elapsed.total_seconds()
        if remaining <= 0:
            raise StopIteration
        return remaining


class JujuResourceTimeout(Exception):
    """A timeout exception for a resource not being downloaded into a unit."""


def pause(seconds):
    print_now('Sleeping for {:d} seconds.'.format(seconds))
    sleep(seconds)


def is_ipv6_address(address):
    """Returns True if address is IPv6 rather than IPv4 or a host name.

    Incorrectly returns False for IPv6 addresses on windows due to lack of
    support for socket.inet_pton there.
    """
    try:
        socket.inet_pton(socket.AF_INET6, address)
    except (AttributeError, socket.error):
        # IPv4 or hostname
        return False
    return True


def split_address_port(address_port):
    """Split an ipv4 or ipv6 address and port into a tuple.

    ipv6 addresses must be in the literal form with a port ([::12af]:80).
    ipv4 addresses may be without a port, which translates to None.
    """
    if ':' not in address_port:
        # This is correct for ipv4.
        return address_port, None
    address, port = address_port.rsplit(':', 1)
    address = address.strip('[]')
    return address, port


def print_now(string):
    print(string)
    sys.stdout.flush()


@contextmanager
def temp_dir(parent=None, keep=False):
    directory = mkdtemp(dir=parent)
    try:
        yield directory
    finally:
        if not keep:
            rmtree(directory)


def check_free_disk_space(path, required, purpose):
    df_result = subprocess.check_output(["df", "-k", path])
    df_result = df_result.split('\n')[1]
    df_result = re.split(' +', df_result)
    available = int(df_result[3])
    if available < required:
        message = (
            "Warning: Probably not enough disk space available for\n"
            "%(purpose)s in directory %(path)s,\n"
            "mount point %(mount)s\n"
            "required: %(required)skB, available: %(available)skB."
            )
        print(message % {
            'path': path, 'mount': df_result[5], 'required': required,
            'available': available, 'purpose': purpose
            })


@contextmanager
def skip_on_missing_file():
    """Skip to the end of block if a missing file exception is raised."""
    try:
        yield
    except (IOError, OSError) as e:
        if e.errno != errno.ENOENT:
            raise


def ensure_dir(path):
    try:
        os.mkdir(path)
    except OSError as e:
        if e.errno != errno.EEXIST:
            raise


def ensure_deleted(path):
    with skip_on_missing_file():
        os.unlink(path)


@contextmanager
def temp_yaml_file(yaml_dict, encoding="utf-8"):
    temp_file_cxt = NamedTemporaryFile(suffix='.yaml', delete=False)
    try:
        with temp_file_cxt as temp_file:
            yaml.safe_dump(yaml_dict, temp_file, encoding=encoding)
        yield temp_file.name
    finally:
        os.unlink(temp_file.name)


def get_timeout_path():
    import jujupy.timeout
    return os.path.abspath(jujupy.timeout.__file__)


def unqualified_model_name(model_name):
    """Return the model name with the owner qualifier stripped if present."""
    return model_name.split('/', 1)[-1]


def qualified_model_name(model_name, owner_name):
    """Return the model name qualified with the given owner name."""
    if model_name == '' or owner_name == '':
        raise ValueError(
            'Neither model_name nor owner_name can be blank strings')

    parts = model_name.split('/', 1)
    if len(parts) == 2 and parts[0] != owner_name:
        raise ValueError(
            'qualified model name {} with owner not matching {}'.format(
                model_name, owner_name))
    return '{}/{}'.format(owner_name, parts[-1])
