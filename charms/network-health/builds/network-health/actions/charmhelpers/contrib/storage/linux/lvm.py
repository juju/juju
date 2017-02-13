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

from subprocess import (
    CalledProcessError,
    check_call,
    check_output,
    Popen,
    PIPE,
)


##################################################
# LVM helpers.
##################################################
def deactivate_lvm_volume_group(block_device):
    '''
    Deactivate any volume gruop associated with an LVM physical volume.

    :param block_device: str: Full path to LVM physical volume
    '''
    vg = list_lvm_volume_group(block_device)
    if vg:
        cmd = ['vgchange', '-an', vg]
        check_call(cmd)


def is_lvm_physical_volume(block_device):
    '''
    Determine whether a block device is initialized as an LVM PV.

    :param block_device: str: Full path of block device to inspect.

    :returns: boolean: True if block device is a PV, False if not.
    '''
    try:
        check_output(['pvdisplay', block_device])
        return True
    except CalledProcessError:
        return False


def remove_lvm_physical_volume(block_device):
    '''
    Remove LVM PV signatures from a given block device.

    :param block_device: str: Full path of block device to scrub.
    '''
    p = Popen(['pvremove', '-ff', block_device],
              stdin=PIPE)
    p.communicate(input='y\n')


def list_lvm_volume_group(block_device):
    '''
    List LVM volume group associated with a given block device.

    Assumes block device is a valid LVM PV.

    :param block_device: str: Full path of block device to inspect.

    :returns: str: Name of volume group associated with block device or None
    '''
    vg = None
    pvd = check_output(['pvdisplay', block_device]).splitlines()
    for l in pvd:
        l = l.decode('UTF-8')
        if l.strip().startswith('VG Name'):
            vg = ' '.join(l.strip().split()[2:])
    return vg


def create_lvm_physical_volume(block_device):
    '''
    Initialize a block device as an LVM physical volume.

    :param block_device: str: Full path of block device to initialize.

    '''
    check_call(['pvcreate', block_device])


def create_lvm_volume_group(volume_group, block_device):
    '''
    Create an LVM volume group backed by a given block device.

    Assumes block device has already been initialized as an LVM PV.

    :param volume_group: str: Name of volume group to create.
    :block_device: str: Full path of PV-initialized block device.
    '''
    check_call(['vgcreate', volume_group, block_device])
