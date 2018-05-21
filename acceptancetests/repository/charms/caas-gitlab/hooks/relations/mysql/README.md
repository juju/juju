# Overview

This interface layer handles the communication with MySQL via the `mysql`
interface protocol.

# Usage

## Requires

The interface layer will set the following states, as appropriate:

  * `{endpoint_name}.connected`  The relation is established, but MySQL has not
    provided the information to use the database
  * `{endpoint_name}.available`  MySQL is ready for use.  You can get the
    connection information via the following methods:

    * `host()`
    * `port()`
    * `database()` (the database name)
    * `user()`
    * `password()`

  * `{endpoint_name}.changed`  The connection info is all available and has
    changed. You should clear this flag in your charm once you have dealt
    with the change.

An example charm using this might look like:

```python
from charmhelpers.core import hookenv
from charms.reactive import when_all, when_any
from charms.reactive import set_flag
from charms.reactive import clear_flag
from charms.reactive import endpoint_from_name
from charms.reactive import data_changed

@when_all('db.available',
          'config.set.admin-pass')
@when_any('db.changed',
          'config.changed.admin-pass')
def render_config():
    mysql = endpoint_from_flag('db.available')
    render_template('app-config.j2', '/etc/app.conf', {
        'admin_pass': hookenv.config('admin-pass'),
        'db_conn': mysql.connection_string(),
    })
    set_flag('charm.needs-restart')
    clear_flag('db.changed')
    clear_flag('config.changed.admin-pass')

@when('charm.needs-restart')
def restart_service():
    hookenv.service_restart('myapp')
    clear_flag('charm.needs-restart')
```
