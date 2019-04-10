package test

import "github.com/zhiqiangxu/urpc"

func main() {
	go startServer()

}

func startServer() {
	urpc.NewServer()
}
