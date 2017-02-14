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

'''
Functions for managing volumes in juju units. One volume is supported per unit.
Subordinates may have their own storage, provided it is on its own partition.

Configuration stanzas::

  volume-ephemeral:
    type: boolean
    default: true
    description: >
      If false, a volume is mounted as sepecified in "volume-map"
      If true, ephemeral storage will be used, meaning that log data
         will only exist as long as the machine. YOU HAVE BEEN WARNED.
  volume-map:
    type: string
    default: {}
    description: >
      YAML map of units to device names, e.g:
        "{ rsyslog/0: /dev/vdb, rsyslog/1: /dev/vdb }"
      Service units will raise a configure-error if volume-ephemeral
      is 'true' and no volume-map value is set. Use 'juju set' to set a
      value and 'juju resolved' to complete configuration.

Usage::

    from charmsupport.volumes import configure_volume, VolumeConfigurationError
    from charmsupport.hookenv import log, ERROR
    def post_mount_hook():
        stop_service('myservice')
    def post_mount_hook():
        start_service('myservice')

    if __name__ == '__main__':
        try:
            configure_volume(before_change=pre_mount_hook,
                             after_change=post_mount_hook)
        except VolumeConfigurationError:
            log('Storage could not be configured', ERROR)

'''

# XXX: Known limitations
# - fstab is neither consulted nor updated

import os
from charmhelpers.core import hookenv
from charmhelpers.core import host
import yaml


MOUNT_BASE = '/srv/juju/volumes'


class VolumeConfigurationError(Exception):
    '''Volume configuration data is missing or invalid'''
    pass


def get_config():
    '''Gather and sanity-check volume configuration data'''
    volume_config = {}
    config = hookenv.config()

    errors = False

    if config.get('volume-ephemeral') in (True, 'True', 'true', 'Yes', 'yes'):
        volume_config['ephemeral'] = True
    else:
        volume_config['ephemeral'] = False

    try:
        volume_map = yaml.safe_load(config.get('volume-map', '{}'))
    except yaml.YAMLError as e:
        hookenv.log("Error parsing YAML volume-map: {}".format(e),
                    hookenv.ERROR)
        errors = True
    if volume_map is None:
        # probably an empty string
        volume_map = {}
    elif not isinstance(volume_map, dict):
        hookenv.log("Volume-map should be a dictionary, not {}".format(
            type(volume_map)))
        errors = True

    volume_config['device'] = volume_map.get(os.environ['JUJU_UNIT_NAME'])
    if volume_config['device'] and volume_config['ephemeral']:
        # asked for ephemeral storage but also defined a volume ID
        hookenv.log('A volume is defined for this unit, but ephemeral '
                    'storage was requested', hookenv.ERROR)
        errors = True
    elif not volume_config['device'] and not volume_config['ephemeral']:
        # asked for permanent storage but did not define volume ID
        hookenv.log('Ephemeral storage was requested, but there is no volume '
                    'defined for this unit.', hookenv.ERROR)
        errors = True

    unit_mount_name = hookenv.local_unit().replace('/', '-')
    volume_config['mountpoint'] = os.path.join(MOUNT_BASE, unit_mount_name)

    if errors:
        return None
    return volume_config


def mount_volume(config):
    if os.path.exists(config['mountpoint']):
        if not os.path.isdir(config['mountpoint']):
            hookenv.log('Not a directory: {}'.format(config['mountpoint']))
            raise VolumeConfigurationError()
    else:
        host.mkdir(config['mountpoint'])
    if os.path.ismount(config['mountpoint']):
        unmount_volume(config)
    if not host.mount(config['device'], config['mountpoint'], persist=True):
        raise VolumeConfigurationError()


def unmount_volume(config):
    if os.path.ismount(config['mountpoint']):
        if not host.umount(config['mountpoint'], persist=True):
            raise VolumeConfigurationError()


def managed_mounts():
    '''List of all mounted managed volumes'''
    return filter(lambda mount: mount[0].startswith(MOUNT_BASE), host.mounts())


def configure_volume(before_change=lambda: None, after_change=lambda: None):
    '''Set up storage (or don't) according to the charm's volume configuration.
       Returns the mount point or "ephemeral". before_change and after_change
       are optional functions to be called if the volume configuration changes.
    '''

    config = get_config()
    if not config:
        hookenv.log('Failed to read volume configuration', hookenv.CRITICAL)
        raise VolumeConfigurationError()

    if config['ephemeral']:
        if os.path.ismount(config['mountpoint']):
            before_change()
            unmount_volume(config)
            after_change()
        return 'ephemeral'
    else:
        # persistent storage
        if os.path.ismount(config['mountpoint']):
            mounts = dict(managed_mounts())
            if mounts.get(config['mountpoint']) != config['device']:
                before_change()
                unmount_volume(config)
                mount_volume(config)
                after_change()
        else:
            before_change()
            mount_volume(config)
            after_change()
        return config['mountpoint']
