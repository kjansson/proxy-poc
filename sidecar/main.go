package main

import (
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
	server, _ := os.LookupEnv("SERVER_ADDRESS") // Address to server part of proxy

	if server == "" {
		log.Fatalln("No server address in enviroment variable SERVER_ADDRESS")
	}

	portEnv, portOk := os.LookupEnv("PROXY_PORT") // Port to bind to locally
	if !portOk {
		portEnv = "161"
		log.Println("Defaulting to port 161.")
	}

	port, err := strconv.Atoi(portEnv)
	if err != nil {
		log.Fatalln("Could not parse port from enviroment variable. Malformed?")
	}

	// Bind and listen for UDP traffic locally
	log.Printf("Binding to 0.0.0.0:%d\n", port)
	udpListener, err = tproxy.ListenUDP("udp", &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: port})
	if err != nil {
		log.Fatalf("Encountered error while binding UDP listener: %s", err)
		return
	}

	// Start listener
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
		n, srcAddr, dstAddr, err := tproxy.ReadFromUDP(udpListener, buff) // Read UDP packet
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				log.Printf("Temporary error while reading data: %s", netErr)
			}

			log.Fatalf("Unrecoverable error while reading data: %s", err)
			return
		}
		go handleUDPConn(buff[:n], srcAddr, dstAddr, udpListener, server) // Handle connection
	}
}

func handleUDPConn(data []byte, srcAddr, dstAddr *net.UDPAddr, localConn *net.UDPConn, server string) {
	log.Printf("Connection %s to %s", srcAddr, dstAddr)

	// Append "ip:port" to the payload, and pad to 21 bytes (max number of bytes needed for xxx.xxx.xxx.xxx:yyyyy)
	// This will be stripped off in the server and used as destination address
	addr := fmt.Sprintf("%-21v", dstAddr.String())
	data = append(data[:], addr[:]...)

	proxyServerConn, err := net.Dial("udp", server) // Dial the server part of the proxy
	if err != nil {
		log.Printf("Failed to connect to original UDP relay server: %s", err)
		return
	}
	defer proxyServerConn.Close()

	_, err = proxyServerConn.Write(data) // Send the wrapped package to the server
	if err != nil {
		log.Printf("Encountered error while writing to remote [%s]: %s", proxyServerConn.RemoteAddr(), err)
		return
	}
	// Wait for response
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
	// Write response back to local socket
	_, err = localConn.WriteToUDP(data, srcAddr)
	if err != nil {
		log.Printf("Error writing to local [%s]: %s", localConn.RemoteAddr(), err)
		return
	}
}
