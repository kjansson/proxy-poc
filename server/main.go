package main

import (
	"bytes"
	"encoding/binary"
	"fmt"
	"log"
	"net"
	"os"
	"reflect"
	"strings"
	"syscall"
	"time"
)

const addrMaxBytes = 21

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

		// Strip the last 21 bytes off the payload, this is the ip+port (padded to 21 bytes) added by the client
		orgAddr := strings.TrimSpace(string(buf[len(buf)-addrMaxBytes:]))
		// Split by ":" to get ip and port
		orgAddrParts := strings.Split(orgAddr, ":")

		udpAddr, err := net.ResolveUDPAddr("udp", orgAddrParts[0]+":"+string(orgAddrParts[1])) // Address is now original destination
		if err != nil {
			log.Println("Error resolving address", err)
		}

		conn, err := net.DialUDP("udp", nil, udpAddr) // Dial
		if err != nil {
			log.Println("Error dialing UDP", err)
			return
		}
		defer conn.Close()

		conn.Write(buf[:len(buf)-21]) // And send the payload, minus the ip+port

		// Wait for response
		data := make([]byte, 1024)
		conn.SetReadDeadline(time.Now().Add(1 * time.Second))
		_, err = conn.Read(data)
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
