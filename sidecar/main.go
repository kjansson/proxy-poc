package main

import (
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"strconv"
	"time"

	tproxy "github.com/LiamHaworth/go-tproxy"
)

var (
	udpListener *net.UDPConn
)

type Wrapper struct {
	Payload []byte
	Ip      string
	Port    int
	Length  int
}

func main() {
	log.Println("Starting proxy client")
	var err error
	server, _ := os.LookupEnv("SERVER_ADDRESS")

	if server == "" {
		log.Fatalln("No server address in enviroment variable SERVER_ADDRESS")
	}

	portEnv, portOk := os.LookupEnv("PROXY_PORT")
	if !portOk {
		portEnv = "161"
		log.Println("Defaulting to port 161.")
	}

	port, err := strconv.Atoi(portEnv)
	if err != nil {
		log.Fatalln("Could not parse port from enviroment variable. Malformed?")
	}

	log.Printf("Binding to 0.0.0.0:%d\n", port)
	udpListener, err = tproxy.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: port})
	if err != nil {
		log.Fatalf("Encountered error while binding UDP listener: %s", err)
		return
	}

	go listenUDP(server)

	interruptListener := make(chan os.Signal)
	signal.Notify(interruptListener, os.Interrupt)
	<-interruptListener
	udpListener.Close()
	log.Println("proxy closing")
}

func listenUDP(server string) {
	for {
		buff := make([]byte, 1024)
		n, srcAddr, dstAddr, err := tproxy.ReadFromUDP(udpListener, buff)
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				log.Printf("Temporary error while reading data: %s", netErr)
			}

			log.Fatalf("Unrecoverable error while reading data: %s", err)
			return
		}
		go handleUDPConn(buff[:n], srcAddr, dstAddr, udpListener, server)
	}
}

func handleUDPConn(data []byte, srcAddr, dstAddr *net.UDPAddr, localConn *net.UDPConn, server string) {
	log.Printf("Connection %s to %s", srcAddr, dstAddr)

	pl := Wrapper{
		Payload: data,
		Ip:      dstAddr.IP.String(),
		Port:    dstAddr.Port,
		Length:  len(data),
	}

	jpl, err := json.Marshal(pl)
	if err != nil {
		fmt.Println("JSON marshal failed", err)
	}
	proxyServerConn, err := net.Dial("udp", server)
	if err != nil {
		log.Printf("Failed to connect to original UDP relay server: %s", err)
		return
	}
	defer proxyServerConn.Close()

	_, err = proxyServerConn.Write(jpl)
	if err != nil {
		log.Printf("Encountered error while writing to remote [%s]: %s", proxyServerConn.RemoteAddr(), err)
		return
	}
	data = make([]byte, 1024)
	proxyServerConn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err = proxyServerConn.Read(data)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return
		}

		log.Printf("Encountered error while reading from remote [%s]: %s", proxyServerConn.RemoteAddr(), err)
		return
	}
	_, err = localConn.WriteToUDP(data, srcAddr)
	if err != nil {
		log.Printf("Error writing to local [%s]: %s", localConn.RemoteAddr(), err)
		return
	}
}
