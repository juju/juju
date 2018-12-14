from charms.reactive import when, when_not
from charms.reactive.flags import set_flag, get_state, clear_flag
from charmhelpers.core.hookenv import (
    log,
    metadata,
    status_set,
    config,
    network_get,
    relation_id,
)


from charms import layer


@when_not('layer.docker-resource.mysql_image.fetched')
def fetch_image():
    layer.docker_resource.fetch('mysql_image')


@when('mysql.configured')
def mariadb_active():
    status_set('active', '')


@when('layer.docker-resource.mysql_image.available')
@when_not('mysql.configured')
def config_mariadb():
    status_set('maintenance', 'Configuring mysql container')

    spec = make_pod_spec()
    log('set pod spec:\n{}'.format(spec))
    layer.caas_base.pod_spec_set(spec)

    set_flag('mysql.configured')


def make_pod_spec():
    with open('reactive/spec_template.yaml') as spec_file:
        pod_spec_template = spec_file.read()

    md = metadata()
    cfg = config()

    user = cfg.get('user')
    set_flag('user', user)
    password = cfg.get('password')
    set_flag('password', password)
    database = cfg.get('database')
    set_flag('database', database)
    root_password = cfg.get('root_password')
    set_flag('root_password', root_password)

    image_info = layer.docker_resource.get_info('mysql_image')

    data = {
        'name': md.get('name'),
        'docker_image_path': image_info.registry_path,
        'docker_image_username': image_info.username,
        'docker_image_password': image_info.password,
        'port': cfg.get('mysql_port'),
        'user': user,
        'password': password,
        'database': database,
        'root_password': root_password,
    }
    data.update(cfg)
    return pod_spec_template % data


@when('mysql.configured')
@when('server.database.requested')
def provide_database(mysql):
    log('db requested')

    for request, application in mysql.database_requests().items():
        log('request -> {0} for app -> {1}'.format(request, application))
        database_name = get_state('database')
        user = get_state('user')
        password = get_state('password')

        log('db params: {0}:{1}@{2}'.format(user, password, database_name))
        info = network_get('server', relation_id())
        log('network info {0}'.format(info))

        mysql.provide_database(
            request_id=request,
            host=info['ingress-addresses'][0],
            port=3306,
            database_name=database_name,
            user=user,
            password=password,
        )
        clear_flag('server.database.requested')
