from golang:1.18.0-bullseye as build
COPY main.go /go/src/proxy/main.go
COPY go.mod /go/src/proxy/go.mod
WORKDIR /go/src/proxy/
RUN go build  -tags netgo -a -v

from alpine:3.15.2
RUN apk add --no-cache libc6-compat iptables tcpdump
COPY --from=build /go/src/proxy/proxy /proxy
ADD scripts /scripts
COPY entrypoint.sh /entrypoint.sh
RUN chmod +x /entrypoint.sh
ENTRYPOINT /entrypoint.sh
