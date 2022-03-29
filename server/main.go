package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"strconv"
	"syscall"
	"time"
)

func rFieldByNames(i interface{}, fields ...string) (field reflect.Value) {
	v := reflect.Indirect(reflect.ValueOf(i))
	for _, n := range fields {
		field = reflect.Indirect(v.FieldByName(n))
		v = field
	}
	return
}
func main() {
	// listen to incoming udp packets

	server_address, sa_ok := os.LookupEnv("SERVER_ADDRESS")

	if !sa_ok {
		log.Println("No server address given.")
		os.Exit(1)
	}

	server_port, sp_ok := os.LookupEnv("SERVER_PORT")
	if !sp_ok {
		log.Println("Defaulting to port 10161.")
		server_port = "10161"
	}

	log.Printf("Binding to %s:%s\n", server_address, server_port)
	udpAddr, err := net.ResolveUDPAddr("udp", server_address+":"+server_port)

	pc, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	fd := int(rFieldByNames(pc, "fd", "pfd", "Sysfd").Int())
	if err = syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1); err != nil { // Allow binding to non-local
		syscall.Close(fd)
		log.Println("Could not set socket option IP_TRANSPARENT")
		syscall.Exit(1)
	}

	if err = syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1); err != nil { // Enable getting original destination address
		log.Println("Could not set socket option IP_RECVORIGDSTADDR")
		syscall.Exit(1)
	}

	for {
		buf := make([]byte, 4096)
		oob := make([]byte, 128)
		n, n2, flags, addr, err := pc.ReadMsgUDP(buf, oob) // Get a message and out-of-bound data
		if err != nil {
			fmt.Println("Error while reading UDP message")
			continue
		}
		go serve(pc, addr, buf[:n], n, oob[:n2], n2, flags) // Serve connection
	}

}

type Wrapper struct {
	Payload []byte
	Ip      string // Original destination
	Port    int
	Length  int
}

func serve(pc *net.UDPConn, addr net.Addr, buf []byte, n int, oob []byte, oobn int, flags int) {

	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn]) // Get data from OOB
	if err != nil {
		fmt.Println("Could not parse message")
		syscall.Exit(1)
	}
	for _, msg := range msgs {

		originalDstRaw := &syscall.RawSockaddrInet4{} // Get original destination
		if err = binary.Read(bytes.NewReader(msg.Data), binary.LittleEndian, originalDstRaw); err != nil {
			log.Println("Could not read message data")
			syscall.Exit(1)
		}

		pkt := Wrapper{}                 // Create a new wrapper for the proxy
		err := json.Unmarshal(buf, &pkt) // Unmarshal the wrapped message received
		if err != nil {
			log.Println("Json marshal failed")
		}

		udpAddr, err := net.ResolveUDPAddr("udp", pkt.Ip+":"+strconv.Itoa(pkt.Port)) // Address is now original destination
		if err != nil {
			log.Println("Error resolving address", err)
		}

		conn, err := net.DialUDP("udp", nil, udpAddr) // Dial
		if err != nil {
			log.Println("Error dialing UDP", err)
			return
		}
		defer conn.Close()

		conn.Write(pkt.Payload[:pkt.Length]) // And send the payload

		// Wait for response
		var data []byte
		data = make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		bytesRead, err := conn.Read(data)
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

		// Write the response back
		_, err = pc.WriteToUDP(data, ludpAddr)
		if err != nil {
			log.Printf("Encountered error while writing to local [%s]: %s", pc.LocalAddr(), err)
			return
		}
		fmt.Printf(".") // Just to see that something has passed through
	}
}

func udpAddrToSocketAddr(addr *net.UDPAddr) (syscall.Sockaddr, error) {
	switch {
	case addr.IP.To4() != nil:
		ip := [4]byte{}
		copy(ip[:], addr.IP.To4())

		return &syscall.SockaddrInet4{Addr: ip, Port: addr.Port}, nil

	default:
		ip := [16]byte{}
		copy(ip[:], addr.IP.To16())

		zoneID, err := strconv.ParseUint(addr.Zone, 10, 32)
		if err != nil {
			return nil, err
		}

		return &syscall.SockaddrInet6{Addr: ip, Port: addr.Port, ZoneId: uint32(zoneID)}, nil
	}
}
