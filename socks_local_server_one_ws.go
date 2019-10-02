package main

import (
	//"io"
	"log"
	"net"
	"net/url"
	"fmt"
	"flag"
	"sync"
	//"time"
	"encoding/binary"
	"github.com/gorilla/websocket"
)

const bufmax = 4096 //最大缓存区大小 
const conn_array_max = 65536 //最大连接数量

type connection struct {	
	status uint32 //0:未连接，1：已连接
	conn net.Conn //连接
}

var (
    conn_ws *websocket.Conn
    conn_array [conn_array_max]connection 
    conn_index uint32
    wsconnected bool  
	lock_conn_ws sync.Mutex 
	//lock_wsconnected sync.Mutex
	//lock_conn_array [conn_array_max]sync.Mutex
	//lock_conn_index sync.Mutex
)

//var addr = flag.String("addr", "gungfusocksweb.cfapps.io:4443", "https service address")
var addr = flag.String("addr", "127.0.0.1:8080", "http service address")

func tun2remote(conn net.Conn, index uint32) {
	var buf [bufmax]byte	
	for {
		if conn == nil || conn_ws == nil {
			//lock_conn_array[index].Lock()
			conn_array[index].status = 0
			//lock_conn_array[index].Unlock()
			return
		}
		binary.BigEndian.PutUint32(buf[0:4], index)
		//lock_conn_array[index].Lock()
		n, err :=conn.Read(buf[4:bufmax])
		//lock_conn_array[index].Unlock()
		if err != nil {
			fmt.Println("index is :", index, "read data err:", err)
			//conn.Close()
			//lock_conn_array[index].Lock()
			conn_array[index].status = 0
			//lock_conn_array[index].Unlock()
			return
		}
		lock_conn_ws.Lock()
		err = conn_ws.WriteMessage(websocket.BinaryMessage, buf[0:n+4])	
		lock_conn_ws.Unlock()	
		if err != nil {
			fmt.Println("index is :", index, "send wsmsg err:", err)
			//conn.Close()
			//lock_conn_array[index].Lock()
			conn_array[index].status = 0
			//lock_conn_array[index].Unlock()
			//conn_ws.Close()
			//conn_ws = nil
			//lock_wsconnected.Lock()
			wsconnected = false
			//lock_wsconnected.Unlock()
		}		
	}
}

func tun2local() {
	for {
		if conn_ws == nil || wsconnected == false {
			//lock_wsconnected.Lock()
			wsconnected = false
			//lock_wsconnected.Unlock()
			return
		}
		_, message, err := conn_ws.ReadMessage() 
		if err != nil {
			fmt.Println("read wsmsg err:", err)
			//lock_wsconnected.Lock()
			//conn_ws.Close()
			wsconnected = false
			//conn_ws = nil
			//lock_wsconnected.Unlock()
			return
		}
		x := binary.BigEndian.Uint32(message[0:4])
		if conn_array[x].conn != nil {			
			_, err := conn_array[x].conn.Write(message[4:])			
			if err != nil {
				fmt.Println("index is :", x, "write data err:", err)
			}
		}		
	}
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
		//time.Sleep(time.Millisecond * 50)
		conn, err := l.Accept()
		if err != nil {
			log.Fatal(err)
		}		
		//u := url.URL{Scheme: "wss", Host: *addr, Path: "/echo"}
		u := url.URL{Scheme: "ws", Host: *addr, Path: "/echo"}
		if wsconnected == false {
			//conn_ws = nil
			//time.Sleep(time.Second)
			conn_ws, _, err = websocket.DefaultDialer.Dial(u.String(), nil)
			if err != nil {
				log.Fatal(err)
				//conn.Close()
				//conn_ws = nil				
			}
			//lock_wsconnected.Lock()
			wsconnected = true
			//lock_wsconnected.Unlock()
			go tun2local()
		}
		//lock_conn_array[conn_index].Lock()
		conn_array[conn_index].status = 1
		conn_array[conn_index].conn = conn
		//lock_conn_array[conn_index].Unlock()
		go tun2remote(conn, conn_index)	
		count := conn_index + conn_array_max
		for i := conn_index; i < count; i++ {
			conn_index = (conn_index + 1) % conn_array_max
			if conn_array[conn_index].status == 0 {
				break
			}
		}
	}
}