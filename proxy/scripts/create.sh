#!/bin/bash

if [[ -z "$PROXY_INTERCEPT_PORT_RANGE" ]]; then
    echo "No intercept port range given."
    exit
fi

#iptables -t mangle -A OUTPUT -p udp --dport $PROXY_INTERCEPT_PORT_RANGE -j MARK --set-mark 1
iptables -t mangle -A OUTPUT -p udp -m multiport --dports $PROXY_INTERCEPT_PORT_RANGE -j MARK --set-mark 1

ip route flush table 100
ip rule add fwmark 1 lookup 100
ip route add local 0.0.0.0/0 dev lo table 100
