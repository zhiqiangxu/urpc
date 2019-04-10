package main

import (
	"log"
	"net"
	"time"

	"github.com/zhiqiangxu/urpc"
)

func main() {
	go startServer()

	time.Sleep(time.Second)

	conn, err := urpc.NewConnection(addr)
	if err != nil {
		panic(err)
	}
	bytes := make([]byte, 1024)
	for {
		log.Println("WritePacket", conn.WritePacket(helloCmd, []byte("hello world")))
		time.Sleep(time.Second)

		packet, err := conn.ReadPacket(bytes)
		log.Println("resp cmd", packet.Cmd, "payload", packet.Payload, "err", err)
	}
}

const (
	helloCmd urpc.Cmd = iota
	helloRespCmd

	addr = "0.0.0.0:8888"
)

func startServer() {
	handler := urpc.NewServeMux()
	handler.HandleFunc(helloCmd, func(writer urpc.PacketWriter, packet urpc.Packet, addr *net.UDPAddr) {
		log.Println("client payload", string(packet.Payload))
		log.Println("server WritePacket", writer.WritePacket(helloRespCmd, []byte("resp from server"), addr))
	})
	bindings := []urpc.ServerBinding{
		urpc.ServerBinding{Addr: addr, Handler: handler}}
	server := urpc.NewServer(bindings)
	log.Println(server.ListenAndServe())
}
