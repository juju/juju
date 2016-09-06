<?php
$wp_ffpc_config = array (
  'localhost' =>
  array (
    'hosts' => '127.0.0.1:11211',
    'expire' => '300',
    'invalidation_method' => '2',
    'prefix_meta' => 'meta-',
    'prefix_data' => 'data-',
    'charset' => 'utf-8',
    'log_info' => 0,
    'log' => '0',
    'cache_type' => 'memcached',
    'cache_loggedin' => 0,
    'nocache_home' => 0,
    'nocache_feed' => 0,
    'nocache_archive' => 0,
    'nocache_single' => 0,
    'nocache_page' => 0,
    'sync_protocols' => 0,
    'persistent' => 0,
    'response_header' => 0,
    'generate_time' => 0,
    'version' => '1.1.1',
  ),
);
include_once ('/var/www/wordpress/wp-content/plugins/wp-ffpc/wp-ffpc-backend.php');
include_once ('/var/www/wordpress/wp-content/plugins/wp-ffpc/wp-ffpc-acache.php');
