package main

import (
	"os"
	"flag"
	"log"
	"net"
	"net/http"
	"fmt"
	"sync"
	"time"
	"encoding/binary"
	"github.com/gorilla/websocket"
)

const bufmax = 4096 //最大缓存区大小
const conn_array_max = 65536 //最大连接数量
type connection struct {	
	status uint32 //0:未握手，1：半握手，2：已握手
	conn net.Conn //连接
	needwrite2ws bool //是否有数据要送到websocket
	data [bufmax]byte //要发送到websocket的数据
	datalenth uint32 //数据长度
}

var (
    conn_ws *websocket.Conn
    conn_array [conn_array_max]connection 
    conn_index uint32
    wsconnected bool  
    lock_conn_ws sync.Mutex 
	//lock_wsconnected sync.Mutex
	//lock_conn_array [conn_array_max]sync.Mutex	
)
var upgrader = websocket.Upgrader{}

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

//遍历数据缓存区，将数据发送至websocket
func sendtows() {
	for {
		if wsconnected == false {
			return
		}
		time.Sleep(time.Millisecond * 2)
		for i := uint32(0); i < conn_array_max; i++ {
			if conn_array[i].needwrite2ws == false {
				continue
			}
			err := conn_ws.WriteMessage(websocket.BinaryMessage, conn_array[i].data[0:conn_array[i].datalenth])
			if err != nil {
				continue
			}
			switch conn_array[i].status {
			case 0: conn_array[i].status = 1
			case 1: conn_array[i].status = 2
			}
			conn_array[i].needwrite2ws = false
		}
	}	
}

//提交发送至websocket请求
func reqtows(connid uint32, data []byte, datalen uint32) {
	//如果之前有数据未发送，则等待
	for conn_array[connid].needwrite2ws {
		time.Sleep(time.Millisecond * 2)
	}
	for i := uint32(0); i < datalen; i++ {
		conn_array[connid].data[i] = data[i]
	}
	//conn_array[connid].data[0:datalen] = data[0:datalen]
	conn_array[connid].datalenth = datalen
	conn_array[connid].needwrite2ws = true
}
 //读取远程服务器数据，并提交至发送websocket请求
func tun2local(conn net.Conn, msgid uint32) {
	var sendbuf [bufmax]byte
	if conn == nil || conn_ws == nil {
		//conn_array[msgid].status = 0
		return
	}
	for {
		binary.BigEndian.PutUint32(sendbuf[0:4], msgid)
		n, err :=conn.Read(sendbuf[0+4:bufmax])
		if err != nil {
			fmt.Println("tun2local read error :", err, "msgid is :", msgid)
			//conn_array[msgid].status = 0
			return
		}
		reqtows(msgid, sendbuf[0:n+4], uint32(n+4))
		//lock_conn_ws.Lock()
		//fmt.Println("lock_conn_ws is locked by tun2local :", msgid)
		//err = conn_ws.WriteMessage(websocket.BinaryMessage, sendbuf[0:n+4])
		//lock_conn_ws.Unlock()
		//time.Sleep(time.Millisecond)
		//fmt.Println("lock_conn_ws is unlocked by tun2local :", msgid)
		//if err != nil {
		//	fmt.Println("tun2local sendws error :", err, "msgid is :", msgid)
		//}		
	}
}

//进行第一处握手，返回包提交至发送websocket请求
func handshake1(message []byte, msgid uint32) {
	var sendbuf [bufmax]byte
	var msg vermsg
	var retmsg vermsgret
	retmsg.ver = 0x05
	retmsg.method = 0xFF
	msg.ver = message[0]
	msg.nmethod = message[1]
	var i uint8
	for i = 0; i < msg.nmethod; i++ {
		msg.methods[i] = message[i+2]
	}	
	if msg.ver != 0x5 {				
		return
	}
	for _, method := range msg.methods {
		if method == 0x00 {
			retmsg.method = method
			break
		}
	}
	binary.BigEndian.PutUint32(sendbuf[0:4], msgid)
	sendbuf[0+4] = retmsg.ver
	sendbuf[1+4] = retmsg.method
	//lock_conn_ws.Lock()
	//fmt.Println("lock_conn_ws is locked by handshake1 :", msgid)
	reqtows(msgid, sendbuf[0:2+4], 2+4)
	//err := conn_ws.WriteMessage(websocket.BinaryMessage, sendbuf[0:2+4])
	//lock_conn_ws.Unlock()
	//fmt.Println("lock_conn_ws is unlocked by handshake1 :", msgid)
	//if err != nil {
	//	log.Println("write:", err)
	//	wsconnected = false
	//	return
	//}
	//conn_array[msgid].status = 1
}

//经常第二次握手，返回包提交至发送webscket请求
func handshake2(message []byte, msgid uint32) {
	var sendbuf [bufmax]byte
	var req reqmsg
	var reqret reqmsgret
	reqret.ver = 0x05
	reqret.atyp = 0x01	
	req.ver = message[0]
	req.cmd = message[1]
	req.rsv = message[2]
	req.atyp = message[3]

	//fmt.Println(req.atyp)
	if req.atyp != 0x01 {
		//地址类型不接受
		reqret.rep = 0x08
	} else {
		req.dstaddr[0] = message[4]
		req.dstaddr[1] = message[5]
		req.dstaddr[2] = message[6]
		req.dstaddr[3] = message[7]
		req.dstport[0] = message[8]
		req.dstport[1] = message[9]
		//fmt.Println(req.dstaddr, req.dstport)
		//构造目标地址和端口
		addrstr := fmt.Sprintf("%d.%d.%d.%d", req.dstaddr[0], req.dstaddr[1], req.dstaddr[2], req.dstaddr[3])
		fmt.Println("dial to :", addrstr)
		port := fmt.Sprintf("%d", uint16(req.dstport[0]) << 8 | uint16(req.dstport[1]))
		fmt.Println(port)
		//执行cmd
		switch req.cmd {
		case 0x01 :
			remote, err := net.Dial("tcp", addrstr + ":" + port) //执行CONNECT CMD
			if err != nil {
				fmt.Println("dial to ", addrstr, "error, err is :", err)
				reqret.rep = 0x03
			} else {				
				reqret.rep = 0x00
				//lock_conn_array[msgid].Lock()
				//fmt.Println("lock_conn_array is locked")
				conn_array[msgid].conn = remote
				//lock_conn_array[msgid].Unlock()
				//fmt.Println("lock_conn_array is unlocked")
				go tun2local(remote, msgid)
			}
		case 0x02 :
			reqret.rep = 0x07
		case 0x03 :
			reqret.rep = 0x07
		}
	}
	binary.BigEndian.PutUint32(sendbuf[0:4], msgid)
	sendbuf[0+4] = reqret.ver
	sendbuf[1+4] = reqret.rep
	sendbuf[2+4] = reqret.rsv
	sendbuf[3+4] = reqret.atyp
	sendbuf[4+4] = reqret.bndaddr[0]
	sendbuf[5+4] = reqret.bndaddr[1]
	sendbuf[6+4] = reqret.bndaddr[2]
	sendbuf[7+4] = reqret.bndaddr[3]
	sendbuf[8+4] = reqret.bndport[0]
	sendbuf[9+4] = reqret.bndport[1]
	//lock_conn_ws.Lock()
	//fmt.Println("lock_conn_ws is locked by handshanke2 :", msgid)
	reqtows(msgid, sendbuf[0:10+4], 10+4)
	//err := conn_ws.WriteMessage(websocket.BinaryMessage, sendbuf[0:10+4])
	//lock_conn_ws.Unlock()
	//fmt.Println("lock_conn_ws is unlocked by handshake2 :", msgid)
	//if err != nil {
	//	log.Println("write:", err)
	//	//conn_ws.Close()
	//	return
	//}
	//lock_conn_array[msgid].Lock()
	//fmt.Println("lock_conn_array is locked")
	//conn_array[msgid].status = 2
	//lock_conn_array[msgid].Unlock()
	//fmt.Println("lock_conn_array is unlocked")
}

//将来自websoket的数据转发至相应的远程服务器
func tun2remote(message []byte, msgid uint32) {
	if conn_array[msgid].conn != nil {
		_, err := conn_array[msgid].conn.Write(message[0:])
		if err != nil {
			fmt.Println("tun2remote write erroe :", err, "msgid is :", msgid)
			conn_array[msgid].status = 0
		}
	}	
}

//创建websocket并读取来自websocket的数据，根据状态作相应处理
func handleconn(w http.ResponseWriter, r *http.Request) {
	//升级至websocket
	var err error
	conn_ws, err = upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		wsconnected = false
		return
	}
	wsconnected = true
	go sendtows()
	for {
		if conn_ws == nil {
			wsconnected = false
			return
		}
		_, message, err := conn_ws.ReadMessage()
		if err != nil {
			log.Println("read:", err)
			wsconnected = false
			return
		}
		x := binary.BigEndian.Uint32(message[0:4])
		switch conn_array[x].status {
		case 0: handshake1(message[4:], x)
		case 1:	handshake2(message[4:], x)
		case 2: tun2remote(message[4:], x)
		}
	}		
}

func main() {
	// Listen on TCP port 8080 on all interfaces.
	port := os.Getenv("PORT")
    var addr string
    if port != "" {
        flag.StringVar(&addr,"addr", ":" + port, "http service address")
    } else {
        flag.StringVar(&addr,"addr", ":8080", "http service address")
    }    
    fmt.Println(addr)
	flag.Parse()
    log.SetFlags(0)    
	http.HandleFunc("/echo", handleconn)
    log.Fatal(http.ListenAndServe(addr, nil))   
}