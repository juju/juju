import errno
import os
import subprocess
import tempfile

from charmhelpers.core.hookenv import log


def pod_spec_set(spec):
    log('set pod spec:\n{}'.format(spec), level='TRACE')
    with tempfile.NamedTemporaryFile(delete=False) as spec_file:
        spec_file.write(spec.encode("utf-8"))
    cmd = ['pod-spec-set', "--file", spec_file.name]

    try:
        ret = subprocess.call(cmd)
        os.remove(spec_file.name)
        if ret == 0:
            return
    except OSError as e:
        if e.errno != errno.ENOENT:
            raise
    log_message = 'pod-spec-set failed'
    log(log_message, level='INFO')


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
