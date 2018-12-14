from charmhelpers.core.hookenv import atexit

from charms import layer


if layer.options.get('status', 'patch-hookenv'):
    layer.status._patch_hookenv()
atexit(layer.status._finalize_status)
