#!/usr/bin/env python3
# Copyright 2022 jtirado
# See LICENSE file for licensing details.
#
# Learn more at: https://juju.is/docs/sdk

"""Charm the service.

Refer to the following post for a quick-start guide that will help you
develop a new k8s charm using the Operator Framework:

    https://discourse.charmhub.io/t/4208
"""

import logging

from ops.charm import CharmBase
from ops.main import main

logger = logging.getLogger(__name__)


class UbuntuCharm(CharmBase):
    """Charm the service."""

    def __init__(self, *args):
        super().__init__(*args)

    
if __name__ == "__main__":
    main(UbuntuCharm)
