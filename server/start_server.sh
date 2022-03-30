#!/bin/sh

DEFAULT_IF=$(ip route | grep default | sed -E 's/.*dev ([0-9a-zA-Z]*).*/\1/g')
SERVER_ADDR=$(ip addr show dev $DEFAULT_IF | grep "inet " | sed -E 's/.* ([0-9]+\.[0-9]+\.[0-9]+\.[0-9]+)\/.*/\1/g')
SERVER_ADDRESS=$SERVER_ADDR ./proxy-server

