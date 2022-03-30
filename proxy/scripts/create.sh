#!/bin/bash

iptables -t mangle -A OUTPUT -p udp --dport $PORT_RANGE -j MARK --set-mark 1

ip route flush table 100
ip rule add fwmark 1 lookup 100
ip route add local 0.0.0.0/0 dev lo table 100
