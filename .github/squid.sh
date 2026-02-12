#!/bin/bash

sudo DEBIAN_FRONTEND=noninteractive apt install -y squid nginx ||
(sudo DEBIAN_FRONTEND=noninteractive apt update -y &&
 sudo DEBIAN_FRONTEND=noninteractive apt install -y squid nginx)

sudo tee /etc/netplan/90-squid.yaml <<'EOF'
network:
  version: 2
  dummy-devices:
    squid:
      addresses:
        - 10.255.255.1/32
EOF
sudo chmod 644 /etc/netplan/90-squid.yaml
sudo netplan apply

sudo tee /etc/squid/squid.conf <<SQUID_EOF
http_port 10.255.255.1:3128
acl localhost src 127.0.0.0/8 ::1 10.255.255.1
acl rfc1918 src 10.0.0.0/8
acl rfc1918 src 172.16.0.0/12
acl rfc1918 src 192.168.0.0/16
acl ipv6_local src fc00::/7
acl ipv6_local src fe80::/10
acl SSL_ports port 443
acl Safe_ports port 80
acl Safe_ports port 443
acl CONNECT method CONNECT
http_access deny !Safe_ports
http_access deny CONNECT !SSL_ports
http_access allow localhost
http_access allow rfc1918
http_access allow ipv6_local
http_access deny all
forward_max_tries 20
connect_timeout 60 seconds
read_timeout 120 seconds
request_timeout 120 seconds
server_persistent_connections on
client_persistent_connections on
dns_retransmit_interval 2 seconds
dns_timeout 60 seconds
access_log stdio:/tmp/squid-access.log squid
cache_log /tmp/squid-cache.log
cache deny all
pid_filename /tmp/squid.pid
SQUID_EOF
sudo chmod 644 /etc/squid/squid.conf

sudo squid -k parse

sudo systemctl enable squid
sudo systemctl restart squid

echo "HTTP_PROXY=http://10.255.255.1:3128"  >> "$GITHUB_ENV"
echo "HTTPS_PROXY=http://10.255.255.1:3128" >> "$GITHUB_ENV"
echo "http_proxy=http://10.255.255.1:3128"  >> "$GITHUB_ENV"
echo "https_proxy=http://10.255.255.1:3128" >> "$GITHUB_ENV"
echo "NO_PROXY=localhost,127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,::1" >> "$GITHUB_ENV"
echo "no_proxy=localhost,127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,::1" >> "$GITHUB_ENV"
sudo snap set system proxy.http="http://10.255.255.1:3128"
sudo snap set system proxy.https="http://10.255.255.1:3128"
sudo snap set system proxy.no-proxy="localhost,127.0.0.0/8,10.0.0.0/8,172.16.0.0/12,192.168.0.0/16,::1"

sudo tee /etc/nginx/nginx.conf <<NGINX_EOF
worker_processes auto;
events {
    worker_connections 1024;
}
http {
    proxy_buffering on;
    proxy_buffer_size 16k;
    proxy_buffers 8193 64k;
    proxy_busy_buffers_size 512m;
    proxy_max_temp_file_size 1024m;
    proxy_temp_path /var/cache/nginx/proxy_temp;
    proxy_connect_timeout 10s;
    proxy_read_timeout 300s;
    proxy_send_timeout 60s;
    upstream goproxy {
        server proxy.golang.org:443;
        keepalive 120;
    }
    server {
        listen 10.255.255.1:8999;
        server_name _;
        location / {
            proxy_pass https://goproxy;
            proxy_ssl_server_name on;
            proxy_ssl_name proxy.golang.org;
            proxy_set_header Host proxy.golang.org;
            proxy_http_version 1.1;
            proxy_set_header Connection "";
            proxy_next_upstream error timeout invalid_header http_500 http_502 http_503 http_504;
            proxy_next_upstream_tries 15;
            proxy_next_upstream_timeout 600s;
        }
    }
}
NGINX_EOF
sudo chmod 644 /etc/nginx/nginx.conf
sudo mkdir -p /var/cache/nginx/proxy_temp
sudo chown www-data:www-data /var/cache/nginx/proxy_temp

sudo nginx -t

sudo systemctl enable nginx
sudo systemctl restart nginx

echo "GOPROXY=http://10.255.255.1:8999,direct" >> "$GITHUB_ENV"
