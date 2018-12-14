from charmhelpers.core import hookenv
from charms.reactive import set_flag, when, when_not, hook, is_flag_set

from charms import layer


@when_not('layer.docker-resource.auto-fetched')
def auto_fetch():
    resources = hookenv.metadata().get('resources', {})
    for name, resource in resources.items():
        is_docker = resource.get('type') == 'oci-image'
        is_auto_fetch = resource.get('auto-fetch', False)
        if is_docker and is_auto_fetch:
            layer.docker_resource.fetch(name)
    set_flag('layer.docker-resource.auto-fetched')


@when('layer.docker-resource.pending')
def fetch():
    layer.docker_resource._fetch()


@hook('upgrade-charm')
def check_updates():
    # The upgrade-charm hook is called for resource updates as well as
    # charm code updates, so force all previously fetched resources to
    # be fetched again (which will set the changed flag, if appropriate).
    resources = hookenv.metadata().get('resources', {})
    for name, resource in resources.items():
        if is_flag_set('layer.docker-resource.{}.fetched'.format(name)):
            layer.docker_resource.fetch(name)
