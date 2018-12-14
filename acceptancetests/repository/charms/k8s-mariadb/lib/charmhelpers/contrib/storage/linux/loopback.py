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

import os
import re
from subprocess import (
    check_call,
    check_output,
)

import six


##################################################
# loopback device helpers.
##################################################
def loopback_devices():
    '''
    Parse through 'losetup -a' output to determine currently mapped
    loopback devices. Output is expected to look like:

        /dev/loop0: [0807]:961814 (/tmp/my.img)

    :returns: dict: a dict mapping {loopback_dev: backing_file}
    '''
    loopbacks = {}
    cmd = ['losetup', '-a']
    devs = [d.strip().split(' ') for d in
            check_output(cmd).splitlines() if d != '']
    for dev, _, f in devs:
        loopbacks[dev.replace(':', '')] = re.search(r'\((\S+)\)', f).groups()[0]
    return loopbacks


def create_loopback(file_path):
    '''
    Create a loopback device for a given backing file.

    :returns: str: Full path to new loopback device (eg, /dev/loop0)
    '''
    file_path = os.path.abspath(file_path)
    check_call(['losetup', '--find', file_path])
    for d, f in six.iteritems(loopback_devices()):
        if f == file_path:
            return d


def ensure_loopback_device(path, size):
    '''
    Ensure a loopback device exists for a given backing file path and size.
    If it a loopback device is not mapped to file, a new one will be created.

    TODO: Confirm size of found loopback device.

    :returns: str: Full path to the ensured loopback device (eg, /dev/loop0)
    '''
    for d, f in six.iteritems(loopback_devices()):
        if f == path:
            return d

    if not os.path.exists(path):
        cmd = ['truncate', '--size', size, path]
        check_call(cmd)

    return create_loopback(path)


def is_mapped_loopback_device(device):
    """
    Checks if a given device name is an existing/mapped loopback device.
    :param device: str: Full path to the device (eg, /dev/loop1).
    :returns: str: Path to the backing file if is a loopback device
    empty string otherwise
    """
    return loopback_devices().get(device, "")
