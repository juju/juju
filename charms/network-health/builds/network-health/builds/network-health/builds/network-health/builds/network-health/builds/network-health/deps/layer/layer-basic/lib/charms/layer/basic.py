import os
import sys
import shutil
from glob import glob
from subprocess import check_call

from charms.layer.execd import execd_preinstall


def lsb_release():
    """Return /etc/lsb-release in a dict"""
    d = {}
    with open('/etc/lsb-release', 'r') as lsb:
        for l in lsb:
            k, v = l.split('=')
            d[k.strip()] = v.strip()
    return d


def bootstrap_charm_deps():
    """
    Set up the base charm dependencies so that the reactive system can run.
    """
    # execd must happen first, before any attempt to install packages or
    # access the network, because sites use this hook to do bespoke
    # configuration and install secrets so the rest of this bootstrap
    # and the charm itself can actually succeed. This call does nothing
    # unless the operator has created and populated $CHARM_DIR/exec.d.
    execd_preinstall()
    # ensure that $CHARM_DIR/bin is on the path, for helper scripts
    os.environ['PATH'] += ':%s' % os.path.join(os.environ['CHARM_DIR'], 'bin')
    venv = os.path.abspath('../.venv')
    vbin = os.path.join(venv, 'bin')
    vpip = os.path.join(vbin, 'pip')
    vpy = os.path.join(vbin, 'python')
    if os.path.exists('wheelhouse/.bootstrapped'):
        activate_venv()
        return
    # bootstrap wheelhouse
    if os.path.exists('wheelhouse'):
        with open('/root/.pydistutils.cfg', 'w') as fp:
            # make sure that easy_install also only uses the wheelhouse
            # (see https://github.com/pypa/pip/issues/410)
            charm_dir = os.environ['CHARM_DIR']
            fp.writelines([
                "[easy_install]\n",
                "allow_hosts = ''\n",
                "find_links = file://{}/wheelhouse/\n".format(charm_dir),
            ])
        apt_install(['python3-pip', 'python3-setuptools', 'python3-yaml'])
        from charms import layer
        cfg = layer.options('basic')
        # include packages defined in layer.yaml
        apt_install(cfg.get('packages', []))
        # if we're using a venv, set it up
        if cfg.get('use_venv'):
            if not os.path.exists(venv):
                series = lsb_release()['DISTRIB_CODENAME']
                if series in ('precise', 'trusty'):
                    apt_install(['python-virtualenv'])
                else:
                    apt_install(['virtualenv'])
                cmd = ['virtualenv', '-ppython3', '--never-download', venv]
                if cfg.get('include_system_packages'):
                    cmd.append('--system-site-packages')
                check_call(cmd)
            os.environ['PATH'] = ':'.join([vbin, os.environ['PATH']])
            pip = vpip
        else:
            pip = 'pip3'
            # save a copy of system pip to prevent `pip3 install -U pip`
            # from changing it
            if os.path.exists('/usr/bin/pip'):
                shutil.copy2('/usr/bin/pip', '/usr/bin/pip.save')
        # need newer pip, to fix spurious Double Requirement error:
        # https://github.com/pypa/pip/issues/56
        check_call([pip, 'install', '-U', '--no-index', '-f', 'wheelhouse',
                    'pip'])
        # install the rest of the wheelhouse deps
        check_call([pip, 'install', '-U', '--no-index', '-f', 'wheelhouse'] +
                   glob('wheelhouse/*'))
        if not cfg.get('use_venv'):
            # restore system pip to prevent `pip3 install -U pip`
            # from changing it
            if os.path.exists('/usr/bin/pip.save'):
                shutil.copy2('/usr/bin/pip.save', '/usr/bin/pip')
                os.remove('/usr/bin/pip.save')
        os.remove('/root/.pydistutils.cfg')
        # flag us as having already bootstrapped so we don't do it again
        open('wheelhouse/.bootstrapped', 'w').close()
        # Ensure that the newly bootstrapped libs are available.
        # Note: this only seems to be an issue with namespace packages.
        # Non-namespace-package libs (e.g., charmhelpers) are available
        # without having to reload the interpreter. :/
        reload_interpreter(vpy if cfg.get('use_venv') else sys.argv[0])


def activate_venv():
    """
    Activate the venv if enabled in ``layer.yaml``.

    This is handled automatically for normal hooks, but actions might
    need to invoke this manually, using something like:

        # Load modules from $CHARM_DIR/lib
        import sys
        sys.path.append('lib')

        from charms.layer.basic import activate_venv
        activate_venv()

    This will ensure that modules installed in the charm's
    virtual environment are available to the action.
    """
    venv = os.path.abspath('../.venv')
    vbin = os.path.join(venv, 'bin')
    vpy = os.path.join(vbin, 'python')
    from charms import layer
    cfg = layer.options('basic')
    if cfg.get('use_venv') and '.venv' not in sys.executable:
        # activate the venv
        os.environ['PATH'] = ':'.join([vbin, os.environ['PATH']])
        reload_interpreter(vpy)


def reload_interpreter(python):
    """
    Reload the python interpreter to ensure that all deps are available.

    Newly installed modules in namespace packages sometimes seemt to
    not be picked up by Python 3.
    """
    os.execle(python, python, sys.argv[0], os.environ)


def apt_install(packages):
    """
    Install apt packages.

    This ensures a consistent set of options that are often missed but
    should really be set.
    """
    if isinstance(packages, (str, bytes)):
        packages = [packages]

    env = os.environ.copy()

    if 'DEBIAN_FRONTEND' not in env:
        env['DEBIAN_FRONTEND'] = 'noninteractive'

    cmd = ['apt-get',
           '--option=Dpkg::Options::=--force-confold',
           '--assume-yes',
           'install']
    check_call(cmd + packages, env=env)


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
