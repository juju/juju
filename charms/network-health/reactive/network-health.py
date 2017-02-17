#!/usr/bin/python
from charms.reactive import when, when_not, set_state


@when_not('network-health.installed')
def install_network_health():
    set_state('network-health.installed')
