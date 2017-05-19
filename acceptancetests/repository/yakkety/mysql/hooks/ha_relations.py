#!/usr/bin/env python

import sys
import os

import lib.utils as utils
import lib.ceph_utils as ceph
import lib.cluster_utils as cluster

# CEPH
DATA_SRC_DST = '/var/lib/mysql'
SERVICE_NAME = os.getenv('JUJU_UNIT_NAME').split('/')[0]
POOL_NAME = SERVICE_NAME
LEADER_RES = 'res_mysql_vip'


def ha_relation_joined():
    vip = utils.config_get('vip')
    vip_iface = utils.config_get('vip_iface')
    vip_cidr = utils.config_get('vip_cidr')
    corosync_bindiface = utils.config_get('ha-bindiface')
    corosync_mcastport = utils.config_get('ha-mcastport')

    if None in [vip, vip_cidr, vip_iface]:
        utils.juju_log('WARNING',
                       'Insufficient VIP information to configure cluster')
        sys.exit(1)

    # Starting configuring resources.
    init_services = {
            'res_mysqld': 'mysql',
        }

    # If the 'ha' relation has been made *before* the 'ceph' relation,
    # it doesn't make sense to make it until after the 'ceph' relation is made
    if not utils.is_relation_made('ceph', 'auth'):
        utils.juju_log('INFO',
                       '*ceph* relation does not exist. '
                       'Not sending *ha* relation data yet')
        return
    else:
        utils.juju_log('INFO',
                       '*ceph* relation exists. Sending *ha* relation data')

        block_storage = 'ceph'

        resources = {
            'res_mysql_rbd': 'ocf:ceph:rbd',
            'res_mysql_fs': 'ocf:heartbeat:Filesystem',
            'res_mysql_vip': 'ocf:heartbeat:IPaddr2',
            'res_mysqld': 'upstart:mysql',
            }

        rbd_name = utils.config_get('rbd-name')
        resource_params = {
            'res_mysql_rbd': 'params name="%s" pool="%s" user="%s" '
                             'secret="%s"' % \
                             (rbd_name, POOL_NAME,
                              SERVICE_NAME, ceph.keyfile_path(SERVICE_NAME)),
            'res_mysql_fs': 'params device="/dev/rbd/%s/%s" directory="%s" '
                            'fstype="ext4" op start start-delay="10s"' % \
                            (POOL_NAME, rbd_name, DATA_SRC_DST),
            'res_mysql_vip': 'params ip="%s" cidr_netmask="%s" nic="%s"' % \
                             (vip, vip_cidr, vip_iface),
            'res_mysqld': 'op start start-delay="5s" op monitor interval="5s"',
            }

        groups = {
            'grp_mysql': 'res_mysql_rbd res_mysql_fs res_mysql_vip res_mysqld',
            }

        for rel_id in utils.relation_ids('ha'):
            utils.relation_set(rid=rel_id,
                               block_storage=block_storage,
                               corosync_bindiface=corosync_bindiface,
                               corosync_mcastport=corosync_mcastport,
                               resources=resources,
                               resource_params=resource_params,
                               init_services=init_services,
                               groups=groups)


def ha_relation_changed():
    clustered = utils.relation_get('clustered')
    if (clustered and cluster.is_leader(LEADER_RES)):
        utils.juju_log('INFO', 'Cluster configured, notifying other services')
        # Tell all related services to start using the VIP
        for r_id in utils.relation_ids('shared-db'):
            utils.relation_set(rid=r_id,
                               db_host=utils.config_get('vip'))


def ceph_joined():
    utils.juju_log('INFO', 'Start Ceph Relation Joined')
    ceph.install()
    utils.juju_log('INFO', 'Finish Ceph Relation Joined')


def ceph_changed():
    utils.juju_log('INFO', 'Start Ceph Relation Changed')
    auth = utils.relation_get('auth')
    key = utils.relation_get('key')
    if None in [auth, key]:
        utils.juju_log('INFO', 'Missing key or auth in relation')
        return

    ceph.configure(service=SERVICE_NAME, key=key, auth=auth)

    if cluster.eligible_leader(LEADER_RES):
        sizemb = int(utils.config_get('block-size')) * 1024
        rbd_img = utils.config_get('rbd-name')
        blk_device = '/dev/rbd/%s/%s' % (POOL_NAME, rbd_img)
        ceph.ensure_ceph_storage(service=SERVICE_NAME, pool=POOL_NAME,
                                 rbd_img=rbd_img, sizemb=sizemb,
                                 fstype='ext4', mount_point=DATA_SRC_DST,
                                 blk_device=blk_device,
                                 system_services=['mysql'])
    else:
        utils.juju_log('INFO',
                       'This is not the peer leader. Not configuring RBD.')
        # Stopping MySQL
        if utils.running('mysql'):
            utils.juju_log('INFO', 'Stopping MySQL...')
            utils.stop('mysql')

    # If 'ha' relation has been made before the 'ceph' relation
    # it is important to make sure the ha-relation data is being
    # sent.
    if utils.is_relation_made('ha'):
        utils.juju_log('INFO',
                       '*ha* relation exists. Making sure the ha'
                       ' relation data is sent.')
        ha_relation_joined()
        return

    utils.juju_log('INFO', 'Finish Ceph Relation Changed')


hooks = {
    "ha-relation-joined": ha_relation_joined,
    "ha-relation-changed": ha_relation_changed,
    "ceph-relation-joined": ceph_joined,
    "ceph-relation-changed": ceph_changed,
}

utils.do_hooks(hooks)
