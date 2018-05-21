Requires: PostgreSQLClient
==========================

Example Usage
-------------

This is what a charm using this relation would look like:

.. code-block:: python

    from charmhelpers.core import hookenv
    from charmhelpers.core.reactive import hook
    from charmhelpers.core.reactive import when
    from charmhelpers.core.reactive import when_file_changed
    from charmhelpers.core.reactive import set_state
    from charmhelpers.core.reactive import remove_state

    @when('db.connected')
    def request_db(pgsql):
        pgsql.set_database('mydb')

    @when('config.changed')
    def check_admin_pass():
        admin_pass = hookenv.config()['admin-pass']
        if admin_pass:
            set_state('admin-pass')
        else:
            remove_state('admin-pass')

    @when('db.master.available', 'admin-pass')
    def render_config(pgsql):
        render_template('app-config.j2', '/etc/app.conf', {
            'db_conn': pgsql.master,
            'admin_pass': hookenv.config('admin-pass'),
        })

    @when_file_changed('/etc/app.conf')
    def restart_service():
        hookenv.service_restart('myapp')


Reference
---------
.. autoclass::
    requires.ConnectionString
    :members:

.. autoclass::
    requires.ConnectionStrings
    :members:

.. autoclass::
    requires.PostgreSQLClient
    :members:
