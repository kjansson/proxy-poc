package main

import (
	"bytes"
	"encoding/binary"
	"encoding/json"
	"fmt"
	"log"
	"net"
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

	service := "192.168.0.110:10161"
	udpAddr, err := net.ResolveUDPAddr("udp", service)

	pc, err := net.ListenUDP("udp", udpAddr)
	if err != nil {
		log.Fatal(err)
	}
	defer pc.Close()

	fd := int(rFieldByNames(pc, "fd", "pfd", "Sysfd").Int())
	if err = syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_TRANSPARENT, 1); err != nil {
		syscall.Close(fd)
		log.Println("Could not set socket option IP_TRANSPARENT")
		syscall.Exit(1)
	}

	if err = syscall.SetsockoptInt(fd, syscall.SOL_IP, syscall.IP_RECVORIGDSTADDR, 1); err != nil {
		log.Println("Could not set socket option IP_RECVORIGDSTADDR")
		syscall.Exit(1)
	}

	for {
		buf := make([]byte, 4096)
		oob := make([]byte, 128)
		n, n2, flags, addr, err := pc.ReadMsgUDP(buf, oob)
		if err != nil {
			fmt.Println("Error while reading UDP message")
			continue
		}
		go serve(pc, addr, buf[:n], n, oob[:n2], n2, flags)
	}

}

type Wrapper struct {
	Payload []byte
	Ip      string
	Port    int
	Length  int
}

func serve(pc *net.UDPConn, addr net.Addr, buf []byte, n int, oob []byte, oobn int, flags int) {

	msgs, err := syscall.ParseSocketControlMessage(oob[:oobn])
	if err != nil {
		fmt.Println("Could not parse message")
		syscall.Exit(1)
	}
	for _, msg := range msgs {

		originalDstRaw := &syscall.RawSockaddrInet4{}
		if err = binary.Read(bytes.NewReader(msg.Data), binary.LittleEndian, originalDstRaw); err != nil {
			log.Println("Could not read message data")
			syscall.Exit(1)
		}

		pkt := Wrapper{}
		err := json.Unmarshal(buf, &pkt)
		if err != nil {
			log.Println("Json marshal failed")
		}

		udpAddr, err := net.ResolveUDPAddr("udp", pkt.Ip+":"+strconv.Itoa(pkt.Port))
		if err != nil {
			log.Println("Error resolving address", err)
		}

		conn, err := net.DialUDP("udp", nil, udpAddr)
		if err != nil {
			log.Println("Error dialing UDP", err)
			return
		}
		defer conn.Close()

		conn.Write(pkt.Payload[:pkt.Length])

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

		laddr := addr.String()
		ludpAddr, err := net.ResolveUDPAddr("udp", laddr)

		bytesWritten, err := pc.WriteToUDP(data, ludpAddr)
		if err != nil {
			log.Printf("Encountered error while writing to local [%s]: %s", pc.LocalAddr(), err)
			return
		} else if bytesWritten < bytesRead {
			log.Printf("Not all bytes [%d < %d] in buffer written to locoal [%s]", bytesWritten, len(data), pc.LocalAddr())
			return
		}
		fmt.Printf(".") // Just to see that something happens
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
