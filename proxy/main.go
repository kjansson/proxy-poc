package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"os/signal"
	"reflect"
	"strconv"
	"strings"
	"syscall"
	"time"
	"unsafe"
)

const addrMaxBytes = 21

var (
	udpListener []*net.UDPConn
)

func rFieldByNames(i interface{}, fields ...string) (field reflect.Value) {
	v := reflect.Indirect(reflect.ValueOf(i))
	for _, n := range fields {
		field = reflect.Indirect(v.FieldByName(n))
		v = field
	}
	return
}

func parsePortRange(raw string) ([]int, error) {

	ports := []int{}

	portsAndRanges := strings.Split(raw, ",")

	for _, par := range portsAndRanges {
		if strings.Contains(par, ":") {
			parts := strings.Split(par, ":")
			if len(parts) > 1 {
				return []int{}, fmt.Errorf("Error while parsing possible range, too many colons.")
			}
			first, err := strconv.Atoi(string(par[0]))
			if err != nil {
				return []int{}, fmt.Errorf("Error while parsing range, possibly not valid int.")
			}
			last, err := strconv.Atoi(string(par[0]))
			if err != nil {
				return []int{}, fmt.Errorf("Error while parsing range, possibly not valid int.")
			}
			if first > last {
				return []int{}, fmt.Errorf("Error while parsing range, first in range bigger than last.")
			}
			for i := first; i < last; i++ {
				ports = append(ports, i)
			}
		} else {
			p, err := strconv.Atoi(string(par))
			if err != nil {
				return []int{}, fmt.Errorf("Error while parsing port number, possibly not valid int.")
			}
			ports = append(ports, p)
		}
	}

	return ports, nil
}

func main() {
	var err error
	var ports []int
	var mode, serverAddress, serverBindAddress, serverPort string
	var ok bool

	// Required
	mode, ok = os.LookupEnv("PROXY_MODE")
	if !ok || (mode != "sidecar" && mode != "server") {
		log.Println("No valid proxy mode given in env variable PROXY_MODE. Options are: 'sidecar' or 'server'")
		os.Exit(1)
	}

	if mode == "server" {
		// Required for server

		serverBindAddress, ok = os.LookupEnv("PROXY_SERVER_BIND_ADDRESS")
		if !ok {
			log.Println("No server bind address given.")
			os.Exit(1)
		}
	}

	if mode == "sidecar" {
		// Required for sidecar
		serverAddress, ok = os.LookupEnv("PROXY_SERVER_ADDRESS")
		if !ok {
			log.Println("No server bind address given.")
			os.Exit(1)
		}
		// Required for sidecar
		interceptPortRange, interceptOk := os.LookupEnv("PROXY_INTERCEPT_PORT_RANGE")
		if !interceptOk && mode == "sidecar" {
			log.Println("No intercept port given.")
			os.Exit(1)
		}

		ports, err = parsePortRange(interceptPortRange)
		if err != nil {
			log.Println("Port range parse error: ", err)
			os.Exit(1)
		}
	}

	if mode == "server" {
		serverPort, ok := os.LookupEnv("PROXY_SERVER_PORT")
		if !ok {
			log.Println("Defaulting to port 11111.")
			serverPort = "11111"
		}

		log.Printf("Binding to %s:%s as server.\n", serverBindAddress, serverPort)
		udpAddr, err := net.ResolveUDPAddr("udp", serverBindAddress+":"+serverPort)
		if err != nil {
			log.Println("Error resolving address", err)
		}

		l, err := net.ListenUDP("udp", udpAddr)
		if err != nil {
			log.Println(err)
			os.Exit(1)
		}
		udpListener = append(udpListener, l)

		defer l.Close()

	} else {

		for _, p := range ports {

			log.Printf("Binding to 0.0.0.0:%d as client\n", p)

			udpAddr := &net.UDPAddr{IP: net.ParseIP("0.0.0.0"), Port: p}
			l, err := net.ListenUDP("udp", udpAddr)
			if err != nil {
				log.Fatal(err)
			}
			udpListener = append(udpListener, l)

			defer l.Close()
		}
	}
	for _, listener := range udpListener {
		fd := int(rFieldByNames(listener, "fd", "pfd", "Sysfd").Int())
		if err = syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1); err != nil { // Allow binding to non-local
			syscall.Close(fd)
			log.Println("Could not set socket option IP_TRANSPARENT")
			syscall.Exit(1)
		}

		if err = syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1); err != nil { // Enable getting original destination address
			log.Println("Could not set socket option IP_RECVORIGDSTADDR")
			syscall.Exit(1)
		}
		go serveUDP(listener, serverAddress, serverPort, mode)
	}

	interruptListener := make(chan os.Signal)
	signal.Notify(interruptListener, os.Interrupt)
	<-interruptListener
	for _, listener := range udpListener {
		listener.Close()
	}

}

func serveUDP(listener *net.UDPConn, server_address string, server_port string, mode string) {
	for {
		data := make([]byte, 1024)
		n, srcAddr, dstAddr, err := ReadUDP(listener, data) // Read UDP packet
		if err != nil {
			if netErr, ok := err.(net.Error); ok && netErr.Temporary() {
				log.Printf("Temporary error while reading data: %s", netErr)
			}

			log.Fatalf("Unrecoverable error while reading data: %s", err)
			return
		}
		go serve(listener, data[:n], srcAddr, dstAddr, server_address, server_port, mode) // Handle connection
	}
}

func serve(listener *net.UDPConn, data []byte, addr net.Addr, dstAddr net.Addr, server_address string, server_port string, mode string) {

	var conn *net.UDPConn
	var udpAddr *net.UDPAddr
	var err error
	if mode == "server" {
		// Strip the last 21 bytes off the payload, this is the ip+port (padded to 21 bytes) added by the client
		orgAddr := strings.TrimSpace(string(data[len(data)-addrMaxBytes:]))
		// Split by ":" to get ip and port
		orgAddrParts := strings.Split(orgAddr, ":")

		udpAddr, err = net.ResolveUDPAddr("udp", orgAddrParts[0]+":"+string(orgAddrParts[1])) // Address is now original destination
		if err != nil {
			log.Println("Error resolving address", err)
		}

		data = data[:len(data)-21]
	} else {
		addr := fmt.Sprintf("%-21v", dstAddr.String())
		data = append(data[:], addr[:]...)

		udpAddr, err = net.ResolveUDPAddr("udp", server_address) // Address is now original destination
		if err != nil {
			log.Println("Error resolving address", err)
		}
	}

	conn, err = net.DialUDP("udp", nil, udpAddr) // Dial
	if err != nil {
		log.Println("Error dialing UDP", err)
		return
	}
	defer conn.Close()

	conn.Write(data) // And send the payload, minus the ip+port
	// Wait for response
	responseData := make([]byte, 1024)
	conn.SetReadDeadline(time.Now().Add(1 * time.Second))
	_, err = conn.Read(responseData)
	if err != nil {
		if netErr, ok := err.(net.Error); ok && netErr.Timeout() {
			return
		}
		log.Printf("Encountered error while reading from remote [%s]: %s", conn.RemoteAddr(), err)
		return
	}

	// Create a connection to the local socket
	laddr := addr.String()
	ludpAddr, err := net.ResolveUDPAddr("udp", laddr)
	if err != nil {
		log.Println("Error resolving address", err)
	}

	// Write the response back
	_, err = listener.WriteToUDP(responseData, ludpAddr)
	if err != nil {
		log.Printf("Encountered error while writing to local [%s]: %s", listener.LocalAddr(), err)
		return
	}
}

// Copied from https://github.com/LiamHaworth/go-tproxy
func ReadUDP(conn *net.UDPConn, b []byte) (int, *net.UDPAddr, *net.UDPAddr, error) {
	oob := make([]byte, 1024)
	n, oobn, _, addr, err := conn.ReadMsgUDP(b, oob)
	if err != nil {
		return 0, nil, nil, err
	}

	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		return 0, nil, nil, fmt.Errorf("parsing socket control message: %s", err)
	}

	var originalDst *net.UDPAddr
	for _, msg := range msgs {
		if msg.Header.Level == syscall.SOL_IP && msg.Header.Type == syscall.IP_RECVORIGDSTADDR {
			originalDstRaw := &syscall.RawSockaddrInet4{}
			if err = binary.Read(bytes.NewReader(msg.Data), binary.LittleEndian, originalDstRaw); err != nil {
				return 0, nil, nil, fmt.Errorf("reading original destination address: %s", err)
			}

			switch originalDstRaw.Family {
			case syscall.AF_INET:
				pp := (*syscall.RawSockaddrInet4)(unsafe.Pointer(originalDstRaw))
				p := (*[2]byte)(unsafe.Pointer(&pp.Port))
				originalDst = &net.UDPAddr{
					IP:   net.IPv4(pp.Addr[0], pp.Addr[1], pp.Addr[2], pp.Addr[3]),
					Port: int(p[0])<<8 + int(p[1]),
				}

			case syscall.AF_INET6:
				pp := (*syscall.RawSockaddrInet6)(unsafe.Pointer(originalDstRaw))
				p := (*[2]byte)(unsafe.Pointer(&pp.Port))
				originalDst = &net.UDPAddr{
					IP:   net.IP(pp.Addr[:]),
					Port: int(p[0])<<8 + int(p[1]),
					Zone: strconv.Itoa(int(pp.Scope_id)),
				}

			default:
				return 0, nil, nil, fmt.Errorf("original destination is an unsupported network family")
			}
		}
	}

	if originalDst == nil {
		return 0, nil, nil, fmt.Errorf("unable to obtain original destination: %s", err)
	}

	return n, addr, originalDst, nil
}
