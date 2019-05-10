# This file is part of JujuPy, a library for driving the Juju CLI.
# Copyright 2013-2019 Canonical Ltd.
#
# This program is free software: you can redistribute it and/or modify it
# under the terms of the Lesser GNU General Public License version 3, as
# published by the Free Software Foundation.
#
# This program is distributed in the hope that it will be useful, but WITHOUT
# ANY WARRANTY; without even the implied warranties of MERCHANTABILITY,
# SATISFACTORY QUALITY, or FITNESS FOR A PARTICULAR PURPOSE.  See the Lesser
# GNU General Public License for more details.
#
# You should have received a copy of the Lesser GNU General Public License
# along with this program.  If not, see <http://www.gnu.org/licenses/>.

# Functionality for handling installed or other juju binaries
# (including paths etc.)


import logging

from .base import (
    K8sProviderType,
    ProviderNotAvailable,
    ProviderNotValid,
)

logger = logging.getLogger(__name__)


class Factory(object):

    def __init__(self):
        self._providers = dict()

    def __getitem__(self, name):
        return self.__getattr__(name)

    def __getattr__(self, name):
        logger.debug('getting provider %s from existing providers %s', name, self.providers)
        try:
            key = K8sProviderType[name]
            return self._providers[key]
        except KeyError:
            raise ProviderNotAvailable("provider {} is not defined".format(name))

    @property
    def providers(self):
        return self._providers.keys()

    def register(self, provider):
        key = provider.name
        if key in self._providers.keys():
            logger.warn(
                "provider %s exists. %s will be replaced",
                key, self._providers[key].name,
            )
        self._providers[key] = provider

    def __iter__(self):
        return iter(self._providers.values())


def register_provider(provider):
    if provider.name not in K8sProviderType.values():
        raise ProviderNotValid()
    providers.register(provider)
    return provider


providers = Factory()
