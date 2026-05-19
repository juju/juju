# Workshop Makefile — convenience targets for host-side operations.

# workshop-dev-lxd-trust exports the workshop client certificate and adds it
# to the host LXD trust store. This enables HTTPS communication between the
# workspace and host LXD.
,PHONY: workshop-dev-lxd-trust
workshop-dev-lxd-trust:
	lxc config trust list --format=yaml | yq '.[] | select(.name == "workshop.crt") | .fingerprint' | xargs lxc config trust remove || true
	workshop exec dev-lxd -- cat /home/workshop/.config/lxc/client.crt > /tmp/workshop.crt && lxc config trust add /tmp/workshop.crt
