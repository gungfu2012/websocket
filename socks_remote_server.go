package main

import (
	"os"
	"flag"
	"log"
	"net"
	"net/http"
	"fmt"
	"github.com/gorilla/websocket"
)

const bufmax = 4096 //最大缓存区大小
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

func tun2local(conn net.Conn, conn_ws *websocket.Conn) {
	var buf [bufmax]byte
	if conn == nil || conn_ws == nil {
		return
	}
	for {
		n, err :=conn.Read(buf[0:bufmax])
		if err != nil {
			fmt.Println(err)
			conn.Close()
			conn_ws.Close()
			return
		}
		conn_ws.WriteMessage(websocket.BinaryMessage, buf[0:n])
	}
}

func tun2remote(conn_ws *websocket.Conn, conn net.Conn) {
	if conn == nil || conn_ws == nil {
		return
	}
	for {
		_, message, err := conn_ws.ReadMessage() 
		if err != nil {
			fmt.Println(err)
			conn_ws.Close()
			conn.Close()
			return
		}
		_, err = conn.Write(message[0:])
	}
}

func handleconn(w http.ResponseWriter, r *http.Request) {
	var sendbuf [bufmax]byte
	//升级至websocket
	conn_ws, err := upgrader.Upgrade(w, r, nil)
	if err != nil {
		log.Print("upgrade:", err)
		return
	}
	//第一次握手
	mt, message, err := conn_ws.ReadMessage()
	if err != nil {
		log.Println("read:", err)
		conn_ws.Close()
		return
	}
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
		conn_ws.Close()
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
	err = conn_ws.WriteMessage(mt, sendbuf[0:2])
	if err != nil {
		log.Println("write:", err)
		conn_ws.Close()
		return
	}
	//第二次握手
	var req reqmsg
	var reqret reqmsgret
	var remote net.Conn
	reqret.ver = 0x05
	reqret.atyp = 0x01
	mt, message, err = conn_ws.ReadMessage()
	if err != nil {
		log.Println("read:", err)
		conn_ws.Close()
		return
	}
	req.ver = message[0]
	req.cmd = message[1]
	req.rsv = message[2]
	req.atyp = message[3]

	fmt.Println(req.atyp)
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
				go tun2remote(conn_ws, remote)
				go tun2local(remote, conn_ws)
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
	err = conn_ws.WriteMessage(mt, sendbuf[0:10])
	if err != nil {
		log.Println("write:", err)
		conn_ws.Close()
		return
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
