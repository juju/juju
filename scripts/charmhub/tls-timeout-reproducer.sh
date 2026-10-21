#!/usr/bin/env bash
set -euo pipefail

CHAIN=JUJU_TLS_REPRO
ARM_CHAIN=JUJU_TLS_ARM

cleanup() {
    while iptables -w -C INPUT -p tcp --sport 443 -j "$CHAIN" 2>/dev/null; do
        iptables -w -D INPUT -p tcp --sport 443 -j "$CHAIN"
    done
    while iptables -w -C OUTPUT -p tcp --dport 443 -j "$CHAIN" 2>/dev/null; do
        iptables -w -D OUTPUT -p tcp --dport 443 -j "$CHAIN"
    done
    for chain in "$CHAIN" "$ARM_CHAIN"; do
        if iptables -w -S "$chain" >/dev/null 2>&1; then
            iptables -w -F "$chain"
            iptables -w -X "$chain"
        fi
    done
}

case "${1:-}" in
    setup|cleanup) ;;
    *) echo "Usage: sudo bash $0 {setup|cleanup}"; exit 1 ;;
esac

if (( EUID != 0 )); then
    echo 'Run with sudo on the controller.' >&2
    exit 1
fi

cleanup
if [[ "$1" == cleanup ]]; then
    exit 0
fi

addresses=$(getent ahostsv4 api.charmhub.io | awk '{print $1}' | sort -u)
[[ -n "$addresses" ]]
trap cleanup ERR
iptables -w -N "$CHAIN"
iptables -w -N "$ARM_CHAIN"
iptables -w -A "$ARM_CHAIN" \
    -m recent --name JUJU_TLS_ONCE --mask 0.0.0.0 --rcheck -j RETURN
iptables -w -A "$ARM_CHAIN" \
    -m recent --name JUJU_TLS_ONCE --mask 0.0.0.0 --set \
    -j CONNMARK --set-xmark 0x40000000/0x40000000

for address in $addresses; do
    iptables -w -A "$CHAIN" -s "$address" \
        -p tcp --tcp-flags SYN,ACK SYN,ACK -j "$ARM_CHAIN"
done
iptables -w -A "$CHAIN" -p tcp --tcp-flags SYN,ACK SYN,ACK -j RETURN
iptables -w -A "$CHAIN" -m connmark --mark 0x40000000/0x40000000 -j DROP
iptables -w -I INPUT 1 -p tcp --sport 443 -j "$CHAIN"
trap - ERR
echo 'Armed: first new Charmhub IPv4 connection stalls; retries pass.'
