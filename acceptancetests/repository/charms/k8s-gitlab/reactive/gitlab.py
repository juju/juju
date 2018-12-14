from charms.reactive import when, when_not
from charms.reactive import endpoint_from_flag
from charms.reactive.flags import set_flag, get_state
from charmhelpers.core.hookenv import (
    log,
    metadata,
    status_set,
    config,
)

from charms import layer


@when_not('layer.docker-resource.gitlab_image.fetched')
@when('gitlab.db.related')
def fetch_image():
    layer.docker_resource.fetch('gitlab_image')


@when_not('gitlab.db.related')
@when_not('gitlab.configured')
def gitlab_blocked():
    status_set('blocked', 'Waiting for database')


@when('gitlab.db.related')
@when('gitlab.configured')
def gitlab_active():
    status_set('active', '')


@when_not('gitlab.configured')
@when('gitlab.db.related')
@when('layer.docker-resource.gitlab_image.available')
def config_gitlab():
    dbcfg = get_state('gitlab.db.config')
    log('got db {0}'.format(dbcfg))

    status_set('maintenance', 'Configuring Gitlab container')

    spec = make_pod_spec(dbcfg)
    log('set pod spec:\n{}'.format(spec))
    layer.caas_base.pod_spec_set(spec)

    set_flag('gitlab.configured')


@when('pgsql.master.available')
@when_not('gitlab.db.related')
def render_db_config(pgsql):
    log('pgsql available')
    log('dbname {0}'.format(pgsql.master['dbname']))
    log('host {0}'.format(pgsql.master['host']))
    log('port {0}'.format(pgsql.master['port']))
    log('user {0}'.format(pgsql.master['user']))
    log('password {0}'.format(pgsql.master['password']))

    set_flag('gitlab.db.config', make_db_config(
        'postgresql',
        pgsql.master['dbname'], pgsql.master['host'], pgsql.master['port'],
        pgsql.master['user'], pgsql.master['password']))
    set_flag('gitlab.db.related')


@when('mysql.available')
@when_not('gitlab.db.related')
def mysql_changed():
    log('mysql available')

    mysql = endpoint_from_flag('mysql.available')
    log('dbname {0}'.format(mysql.database()))
    log('host {0}'.format(mysql.host()))
    log('port {0}'.format(mysql.port()))
    log('user {0}'.format(mysql.user()))
    log('password {0}'.format(mysql.password()))

    set_flag('gitlab.db.config', make_db_config(
        'mysql2',
        mysql.database(), mysql.host(), mysql.port(),
        mysql.user(), mysql.password()))
    set_flag('gitlab.db.related')


def make_db_config(dbadaptor, dbname, host, port, user, password):
    cfg_terms = []

    def add_config(cfg_terms, k, v):
        cfg_terms += ['{}={}'.format(k, format_config_value(v))]

    add_config(cfg_terms, 'postgresql[\'enable\']', False)
    add_config(cfg_terms, 'gitlab_rails[\'db_adapter\']', dbadaptor)
    add_config(cfg_terms, 'gitlab_rails[\'db_encoding\']', 'utf8')
    add_config(cfg_terms, 'gitlab_rails[\'db_database\']', dbname)
    add_config(cfg_terms, 'gitlab_rails[\'db_host\']', host)
    add_config(cfg_terms, 'gitlab_rails[\'db_port\']', port)
    add_config(cfg_terms, 'gitlab_rails[\'db_username\']', user)
    add_config(cfg_terms, 'gitlab_rails[\'db_password\']', password)

    return '; '.join(map(str, cfg_terms))


def make_pod_spec(dbcfg):
    image_info = layer.docker_resource.get_info('gitlab_image')

    with open('reactive/spec_template.yaml') as spec_file:
        pod_spec_template = spec_file.read()

    md = metadata()
    cfg = config()
    data = {
        'name': md.get('name'),
        'docker_image_path': image_info.registry_path,
        'docker_image_username': image_info.username,
        'docker_image_password': image_info.password,
        'port': cfg.get('http_port'),
        'config': '; '.join([compose_config(cfg), dbcfg])
    }
    return pod_spec_template % data


def isfloat(value):
    try:
        float(value)
        return True
    except ValueError:
        return False


def format_config_value(value):
    val = str(value)
    if isinstance(value, bool):
        if value:
            val = "true"
        else:
            val = "false"
    elif val.isdigit():
        val = int(val)
    elif isfloat(val):
        val = float(val)
    else:
        val = "'{}'".format(val)
    return val


def compose_config(cfg):
    exturl = None

    if cfg.get('external_url'):
        exturl = cfg.get('external_url')
        if exturl != '' and not exturl.startswith("http"):
            exturl = "http://" + exturl

    http_port = cfg.get('http_port')
    if exturl is not None and http_port is not None:
        if exturl.endswith("/"):
            exturl = exturl[:-1]

        exturl = exturl + ":{}".format(http_port)

    cfg_terms = []
    if exturl is not None:
        cfg_terms = ['external_url {}'.format(format_config_value(exturl))]

    def maybe_add_config(cfg_terms, k, v):
        if v is None or str(v) == '':
            return
        cfg_terms += ['{}={}'.format(k, format_config_value(v))]

    maybe_add_config(cfg_terms, 'gitlab_rails[\'gitlab_ssh_host\']',
                     cfg.get('ssh_host')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'time_zone\']',
                     cfg.get('time_zone')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'gitlab_email_from\']',
                     cfg.get('email_from')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'gitlab_email_display_name\']',
                     cfg.get('from_email_name')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'gitlab_email_reply_to\']',
                     cfg.get('reply_to_email')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'smtp_enable\']',
                     cfg.get('smtp_enable')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'smtp_address\']',
                     cfg.get('smtp_address')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'smtp_port\']',
                     cfg.get('smtp_port')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'smtp_user_name\']',
                     cfg.get('smtp_user_name')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'smtp_password\']',
                     cfg.get('smtp_password')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'smtp_domain\']',
                     cfg.get('smtp_domain')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'smtp_enable_starttls_auto\']',
                     cfg.get('smtp_enable_starttls_auto')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'smtp_tls\']',
                     cfg.get('smtp_tls')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_enabled\']',
                     cfg.get('incoming_email_enabled')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_address\']',
                     cfg.get('incoming_email_address')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_email\']',
                     cfg.get('incoming_email_email')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_password\']',
                     cfg.get('incoming_email_password')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_host\']',
                     cfg.get('incoming_email_host')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_port\']',
                     cfg.get('incoming_email_port')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_ssl\']',
                     cfg.get('incoming_email_ssl')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_start_tls\']',
                     cfg.get('incoming_email_start_tls')),
    maybe_add_config(cfg_terms, 'gitlab_rails[\'incoming_email_mailbox_name\']',
                     cfg.get('incoming_email_mailbox_name')),

    return '; '.join(map(str, cfg_terms))
