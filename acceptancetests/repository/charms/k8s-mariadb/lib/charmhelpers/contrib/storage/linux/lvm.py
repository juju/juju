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

import functools
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
    for lvm in pvd:
        lvm = lvm.decode('UTF-8')
        if lvm.strip().startswith('VG Name'):
            vg = ' '.join(lvm.strip().split()[2:])
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


def list_logical_volumes(select_criteria=None, path_mode=False):
    '''
    List logical volumes

    :param select_criteria: str: Limit list to those volumes matching this
                                 criteria (see 'lvs -S help' for more details)
    :param path_mode: bool: return logical volume name in 'vg/lv' format, this
                            format is required for some commands like lvextend
    :returns: [str]: List of logical volumes
    '''
    lv_diplay_attr = 'lv_name'
    if path_mode:
        # Parsing output logic relies on the column order
        lv_diplay_attr = 'vg_name,' + lv_diplay_attr
    cmd = ['lvs', '--options', lv_diplay_attr, '--noheadings']
    if select_criteria:
        cmd.extend(['--select', select_criteria])
    lvs = []
    for lv in check_output(cmd).decode('UTF-8').splitlines():
        if not lv:
            continue
        if path_mode:
            lvs.append('/'.join(lv.strip().split()))
        else:
            lvs.append(lv.strip())
    return lvs


list_thin_logical_volume_pools = functools.partial(
    list_logical_volumes,
    select_criteria='lv_attr =~ ^t')

list_thin_logical_volumes = functools.partial(
    list_logical_volumes,
    select_criteria='lv_attr =~ ^V')


def extend_logical_volume_by_device(lv_name, block_device):
    '''
    Extends the size of logical volume lv_name by the amount of free space on
    physical volume block_device.

    :param lv_name: str: name of logical volume to be extended (vg/lv format)
    :param block_device: str: name of block_device to be allocated to lv_name
    '''
    cmd = ['lvextend', lv_name, block_device]
    check_call(cmd)


def create_logical_volume(lv_name, volume_group, size=None):
    '''
    Create a new logical volume in an existing volume group

    :param lv_name: str: name of logical volume to be created.
    :param volume_group: str: Name of volume group to use for the new volume.
    :param size: str: Size of logical volume to create (100% if not supplied)
    :raises subprocess.CalledProcessError: in the event that the lvcreate fails.
    '''
    if size:
        check_call([
            'lvcreate',
            '--yes',
            '-L',
            '{}'.format(size),
            '-n', lv_name, volume_group
        ])
    # create the lv with all the space available, this is needed because the
    # system call is different for LVM
    else:
        check_call([
            'lvcreate',
            '--yes',
            '-l',
            '100%FREE',
            '-n', lv_name, volume_group
        ])
