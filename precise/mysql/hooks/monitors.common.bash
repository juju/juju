MYSQL="mysql -uroot -p`cat /var/lib/mysql/mysql.passwd`"
monitor_user=monitors
. /usr/share/charm-helper/sh/net.sh
if [ -n "$JUJU_REMOTE_UNIT" ] ; then
    remote_addr=$(ch_get_ip $(relation-get private-address))
fi
mkdir -p data
revoke_todo=data/${JUJU_RELATION_ID}
