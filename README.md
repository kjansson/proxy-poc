# udp-proxy-ish
An UDP proxy(-ish) for proxying UDP traffic out of K8s. It is a sidecar-server setup which is 100% transparent to client applications.
A sidecar is deployed to the application which intercepts outgoing UDP traffic on selected ports, and forwards it to an external proxy that can be placed wherever you like. The traffic is then forwarded to its destination and back.


## Configuration

The proxy and sidecar are configured via environment variables.

### Environment variables

```
PROXY_MODE - Required. Role of the container. Can be "proxy" or "sidecar"
PROXY_SERVER_ADDRESS - Required for sidecar. This is the address of the proxy server side, given in the "ip:port" format.
PROXY_SERVER_BIND_ADDRESS - Required for server. This is the address to bind to for the server side of the proxy.
PROXY_SERVER_PORT - Used only in proxy role. Port that the proxy will bind to. Defaults to 11111.
PROXY_INTERCEPT_PORT_RANGE - Required for sidecar. Ports and port ranges that will be intercepted by the sidecar and sent to the proxy. The format is the same as used in iptables --dports option. Examples: "161", "161,162", "161,2002:2005"
```

## Starting the Proxy
```
docker run -it --network host -e PROXY_SERVER_BIND_ADDRESS=xxx.xxx.xxx.xxx -e PROXY_MODE=server proxy-image
```

## Setting up the sidecar in K8s
The sidecar is deployed in the pod where the UDP traffic originates from. It needs the CAP_NET_ADMIN capability to set the correct socket options, and setting up iptables and routing.

Example for proxying SNMP traffic;
```
apiVersion: apps/v1
kind: Deployment
metadata:
  name: proxy-test
  namespace: default
  labels:
    app: proxy-example
spec:
  replicas: 1
  selector:
    matchLabels:
      app: proxy-example
  template:
    metadata:
      labels:
        app: proxy-example
        environment: test
    spec:
      containers:
      - name: client
        image: elcolio/net-snmp:latest
        resources:
          requests:
            cpu: 100m
            memory: 300M
          limits:
            cpu: 100m
            memory: 400M
      - name: proxy-sidecar
        image: proxy-image:tag
        imagePullPolicy: Always
        securityContext:
          capabilities:
            add: ["NET_ADMIN"]
        env:
          - name: PROXY_SERVER_ADDRESS
            value: "10.10.10.15:11111"
          - name: PROXY_MODE
            value: sidecar
          - name: PROXY_INTERCEPT_PORT_RANGE
            value: "161"
        resources:
          requests:
            cpu: 100m
            memory: 300M
          limits:
            cpu: 100m
            memory: 400M

```