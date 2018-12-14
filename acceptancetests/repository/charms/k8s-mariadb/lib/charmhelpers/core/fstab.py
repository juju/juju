#!/usr/bin/env python
# -*- coding: utf-8 -*-

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

import io
import os

__author__ = 'Jorge Niedbalski R. <jorge.niedbalski@canonical.com>'


class Fstab(io.FileIO):
    """This class extends file in order to implement a file reader/writer
    for file `/etc/fstab`
    """

    class Entry(object):
        """Entry class represents a non-comment line on the `/etc/fstab` file
        """
        def __init__(self, device, mountpoint, filesystem,
                     options, d=0, p=0):
            self.device = device
            self.mountpoint = mountpoint
            self.filesystem = filesystem

            if not options:
                options = "defaults"

            self.options = options
            self.d = int(d)
            self.p = int(p)

        def __eq__(self, o):
            return str(self) == str(o)

        def __str__(self):
            return "{} {} {} {} {} {}".format(self.device,
                                              self.mountpoint,
                                              self.filesystem,
                                              self.options,
                                              self.d,
                                              self.p)

    DEFAULT_PATH = os.path.join(os.path.sep, 'etc', 'fstab')

    def __init__(self, path=None):
        if path:
            self._path = path
        else:
            self._path = self.DEFAULT_PATH
        super(Fstab, self).__init__(self._path, 'rb+')

    def _hydrate_entry(self, line):
        # NOTE: use split with no arguments to split on any
        #       whitespace including tabs
        return Fstab.Entry(*filter(
            lambda x: x not in ('', None),
            line.strip("\n").split()))

    @property
    def entries(self):
        self.seek(0)
        for line in self.readlines():
            line = line.decode('us-ascii')
            try:
                if line.strip() and not line.strip().startswith("#"):
                    yield self._hydrate_entry(line)
            except ValueError:
                pass

    def get_entry_by_attr(self, attr, value):
        for entry in self.entries:
            e_attr = getattr(entry, attr)
            if e_attr == value:
                return entry
        return None

    def add_entry(self, entry):
        if self.get_entry_by_attr('device', entry.device):
            return False

        self.write((str(entry) + '\n').encode('us-ascii'))
        self.truncate()
        return entry

    def remove_entry(self, entry):
        self.seek(0)

        lines = [l.decode('us-ascii') for l in self.readlines()]

        found = False
        for index, line in enumerate(lines):
            if line.strip() and not line.strip().startswith("#"):
                if self._hydrate_entry(line) == entry:
                    found = True
                    break

        if not found:
            return False

        lines.remove(line)

        self.seek(0)
        self.write(''.join(lines).encode('us-ascii'))
        self.truncate()
        return True

    @classmethod
    def remove_by_mountpoint(cls, mountpoint, path=None):
        fstab = cls(path=path)
        entry = fstab.get_entry_by_attr('mountpoint', mountpoint)
        if entry:
            return fstab.remove_entry(entry)
        return False

    @classmethod
    def add(cls, device, mountpoint, filesystem, options=None, path=None):
        return cls(path=path).add_entry(Fstab.Entry(device,
                                                    mountpoint, filesystem,
                                                    options=options))
