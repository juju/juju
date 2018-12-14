# Copyright 2017 Canonical Limited.
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
import json

from charmhelpers.core.hookenv import log

stats_intervals = ['stats_day', 'stats_five_minute',
                   'stats_hour', 'stats_total']

SYSFS = '/sys'


class Bcache(object):
    """Bcache behaviour
    """

    def __init__(self, cachepath):
        self.cachepath = cachepath

    @classmethod
    def fromdevice(cls, devname):
        return cls('{}/block/{}/bcache'.format(SYSFS, devname))

    def __str__(self):
        return self.cachepath

    def get_stats(self, interval):
        """Get cache stats
        """
        intervaldir = 'stats_{}'.format(interval)
        path = "{}/{}".format(self.cachepath, intervaldir)
        out = dict()
        for elem in os.listdir(path):
            out[elem] = open('{}/{}'.format(path, elem)).read().strip()
        return out


def get_bcache_fs():
    """Return all cache sets
    """
    cachesetroot = "{}/fs/bcache".format(SYSFS)
    try:
        dirs = os.listdir(cachesetroot)
    except OSError:
        log("No bcache fs found")
        return []
    cacheset = set([Bcache('{}/{}'.format(cachesetroot, d)) for d in dirs if not d.startswith('register')])
    return cacheset


def get_stats_action(cachespec, interval):
    """Action for getting bcache statistics for a given cachespec.
    Cachespec can either be a device name, eg. 'sdb', which will retrieve
    cache stats for the given device, or 'global', which will retrieve stats
    for all cachesets
    """
    if cachespec == 'global':
        caches = get_bcache_fs()
    else:
        caches = [Bcache.fromdevice(cachespec)]
    res = dict((c.cachepath, c.get_stats(interval)) for c in caches)
    return json.dumps(res, indent=4, separators=(',', ': '))
