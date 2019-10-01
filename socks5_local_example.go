package main

import (
	//"io"
	"log"
	"net"
	"fmt"
)

const bufmax = 4096 //最大缓存区大小

type vermsg struct {
	ver uint8
	nmethod uint8
	methods [255]uint8
} //定义socks5版本包-接收

type vermsgret struct {
	ver uint8
	method uint8
} //定义socks5版本包-发送

type reqmsg struct {
	ver uint8
	cmd uint8
	rsv uint8
	atyp uint8
	dstaddr [4]uint8
	dstport [2]uint8
} //定义socks5请求包-接收

type reqmsgret struct {
	ver uint8
	rep uint8
	rsv uint8
	atyp uint8
	bndaddr [4]uint8
	bndport [2]uint8
} //定义socks5请求包-发送

func tun(in net.Conn, out net.Conn) {
	if in == nil || out == nil {
		return
	}
	var buf [bufmax]byte
	for {
		n, err := in.Read(buf[0:bufmax])
		if err != nil {
			in.Close()
			fmt.Println("we got data count :", n, err)
			return
		}
		//fmt.Println(buf[0:n])
		n, err = out.Write(buf[0:n])
		if err != nil {
			out.Close()
			fmt.Println("we sned data count :", n, err)
			return
		}
	}
}

func handleconn(conn net.Conn) {
	var recvbuf [bufmax]byte
	var sendbuf [bufmax]byte

	//第一次握手
	n, err := conn.Read(recvbuf[0:])
	fmt.Println(n, err)
	var msg vermsg
	var retmsg vermsgret
	retmsg.ver = 0x05
	retmsg.method = 0xFF

	msg.ver = recvbuf[0]
	msg.nmethod = recvbuf[1]
	var i uint8
	for i = 0; i < msg.nmethod; i++ {
		msg.methods[i] = recvbuf[i+2]
	}
	
	if msg.ver != 0x5 {
		conn.Close()
		return
	}

	for _, method := range msg.methods {
		if method == 0x00 {
			retmsg.method = method
			break
		}
	}

	sendbuf[0] = retmsg.ver
	sendbuf[1] = retmsg.method
	conn.Write(sendbuf[0:2])

	//第二次握手
	var req reqmsg
	var reqret reqmsgret
	var remote net.Conn

	reqret.ver = 0x05
	reqret.atyp = 0x01

	n, err = conn.Read(recvbuf[0:])
	fmt.Println(n, err)
	req.ver = recvbuf[0]
	req.cmd = recvbuf[1]
	req.rsv = recvbuf[2]
	req.atyp = recvbuf[3]

	fmt.Println(req.atyp)
	if req.atyp != 0x01 {
		//地址类型不接受
		reqret.rep = 0x08
	} else {
		req.dstaddr[0] = recvbuf[4]
		req.dstaddr[1] = recvbuf[5]
		req.dstaddr[2] = recvbuf[6]
		req.dstaddr[3] = recvbuf[7]
		req.dstport[0] = recvbuf[8]
		req.dstport[1] = recvbuf[9]
		fmt.Println(req.dstaddr, req.dstport)
		//构造目标地址和端口
		addrstr := fmt.Sprintf("%d.%d.%d.%d", req.dstaddr[0], req.dstaddr[1], req.dstaddr[2], req.dstaddr[3])
		fmt.Println("this is :", addrstr)
		port := fmt.Sprintf("%d", uint16(req.dstport[0]) << 8 | uint16(req.dstport[1]))
		fmt.Println(port)
		//执行cmd
		switch req.cmd {
		case 0x01 :
			remote, err = net.Dial("tcp", addrstr + ":" + port) //执行CONNECT CMD
			if err != nil {
				fmt.Println(err)
				reqret.rep = 0x03
			} else {
				reqret.rep = 0x00
				//localaddr := remote.LocalAddr()
				//tcpaddr, _ := net.ResolveTCPAddr("tcp", localaddr.String())
				//reqret.bndaddr[0] = tcpaddr.IP[12]
				//reqret.bndaddr[1] = tcpaddr.IP[13]
				//reqret.bndaddr[2] = tcpaddr.IP[14]
				//reqret.bndaddr[3] = tcpaddr.IP[15]
				//fmt.Println("local addr is :", tcpaddr.IP[12], tcpaddr.IP[13], tcpaddr.IP[14], tcpaddr.IP[15])
				//fmt.Println("local port is :", tcpaddr.Port)
				//reqret.bndport[0] = uint8(tcpaddr.Port >> 8)
				//reqret.bndport[1] = uint8(tcpaddr.Port)
				//fmt.Println(reqret.bndport)
				go tun(conn, remote)
				go tun(remote, conn)
			}
		case 0x02 :
			reqret.rep = 0x07
		case 0x03 :
			reqret.rep = 0x07
		}
	}
	sendbuf[0] = reqret.ver
	sendbuf[1] = reqret.rep
	sendbuf[2] = reqret.rsv
	sendbuf[3] = reqret.atyp
	sendbuf[4] = reqret.bndaddr[0]
	sendbuf[5] = reqret.bndaddr[1]
	sendbuf[6] = reqret.bndaddr[2]
	sendbuf[7] = reqret.bndaddr[3]
	sendbuf[8] = reqret.bndport[0]
	sendbuf[9] = reqret.bndport[1]
	conn.Write(sendbuf[0:10])
	//go tun(conn, remote)
	//go tun(remote, conn)
}

func main() {
	// Listen on TCP port 1080 on all interfaces.
	l, err := net.Listen("tcp", ":1080")
	if err != nil {
		log.Fatal(err)
	}
	defer l.Close()
	for {
		// Wait for a connection.
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}
		// Handle the connection in a new goroutine.
		// The loop then returns to accepting, so that
		// multiple connections may be served concurrently.
		go handleconn(conn)
	}
}