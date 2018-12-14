import errno
import os
import subprocess
import tempfile
import yaml
from importlib import import_module
from pathlib import Path

from charmhelpers.core.hookenv import log


def pod_spec_set(spec):
    if not isinstance(spec, str):
        spec = yaml.dump(spec)
    with tempfile.NamedTemporaryFile(delete=False) as spec_file:
        spec_file.write(spec.encode("utf-8"))
    cmd = ['pod-spec-set', "--file", spec_file.name]

    try:
        ret = subprocess.call(cmd)
        os.remove(spec_file.name)
        if ret == 0:
            return True
    except OSError as e:
        if e.errno != errno.ENOENT:
            raise
    log_message = 'pod-spec-set failed'
    log(log_message, level='INFO')
    return False


def init_config_states():
    import yaml
    from charmhelpers.core import hookenv
    from charms.reactive import set_state
    from charms.reactive import toggle_state

    config = hookenv.config()

    config_defaults = {}
    config_defs = {}
    config_yaml = os.path.join(hookenv.charm_dir(), 'config.yaml')
    if os.path.exists(config_yaml):
        with open(config_yaml) as fp:
            config_defs = yaml.safe_load(fp).get('options', {})
            config_defaults = {key: value.get('default')
                               for key, value in config_defs.items()}
    for opt in config_defs.keys():
        if config.changed(opt):
            set_state('config.changed')
            set_state('config.changed.{}'.format(opt))
        toggle_state('config.set.{}'.format(opt), config.get(opt))
        toggle_state('config.default.{}'.format(opt),
                     config.get(opt) == config_defaults[opt])
    hookenv.atexit(clear_config_states)


def clear_config_states():
    from charmhelpers.core import hookenv, unitdata
    from charms.reactive import remove_state

    config = hookenv.config()

    remove_state('config.changed')
    for opt in config.keys():
        remove_state('config.changed.{}'.format(opt))
        remove_state('config.set.{}'.format(opt))
        remove_state('config.default.{}'.format(opt))
    unitdata.kv().flush()


def import_layer_libs():
    """
    Ensure that all layer libraries are imported.

    This makes it possible to do the following:

        from charms import layer

        layer.foo.do_foo_thing()

    Note: This function must be called after bootstrap.
    """
    for module_file in Path('lib/charms/layer').glob('*'):
        module_name = module_file.stem
        if module_name in ('__init__', 'caas_base', 'execd') or not (
            module_file.suffix == '.py' or module_file.is_dir()
        ):
            continue
        import_module('charms.layer.{}'.format(module_name))
