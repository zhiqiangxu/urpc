package urpc

import (
	"errors"
	"net"
	"runtime"
	"sync"
	"time"

	"github.com/zhiqiangxu/qrpc"
)

// ServerBinding contains binding infos
type ServerBinding struct {
	Addr                string
	Handler             Handler // handler to invoke
	DefaultReadTimeout  int
	DefaultWriteTimeout int
	MaxPacketSize       int
}

// PacketWriter for urpc
type PacketWriter interface {
	WritePacket(Cmd, []byte, *net.UDPAddr) error
}

type packetWriter struct {
	udpConn      *net.UDPConn
	writeTimeout int
}

func (w *packetWriter) WritePacket(cmd Cmd, payload []byte, addr *net.UDPAddr) (err error) {
	bytes, err := encodePacket(cmd, payload)
	if err != nil {
		return
	}
	if w.writeTimeout > 0 {
		endTime := time.Now().Add(time.Duration(w.writeTimeout) * time.Second)
		err = w.udpConn.SetWriteDeadline(endTime)
		if err != nil {
			return
		}
	}

	nbytes, err := w.udpConn.WriteToUDP(bytes, addr)
	if err != nil {
		return
	}

	if nbytes != len(bytes) {
		err = errShortWrite
	}

	return
}

// Handler for urpc
type Handler interface {
	ServeURPC(PacketWriter, Packet, *net.UDPAddr)
}

// The HandlerFunc type is an adapter to allow the use of
// ordinary functions as urpc handlers. If f is a function
// with the appropriate signature, HandlerFunc(f) is a
// Handler that calls f.
type HandlerFunc func(PacketWriter, Packet, *net.UDPAddr)

// ServeURPC calls f(w, p).
func (f HandlerFunc) ServeURPC(w PacketWriter, p Packet, addr *net.UDPAddr) {
	f(w, p, addr)
}

// ServeMux is urpc request multiplexer.
type ServeMux struct {
	mu sync.RWMutex
	m  map[Cmd]Handler
}

// NewServeMux allocates and returns a new ServeMux.
func NewServeMux() *ServeMux { return &ServeMux{} }

// HandleFunc registers the handler function for the given pattern.
func (mux *ServeMux) HandleFunc(cmd Cmd, handler func(PacketWriter, Packet, *net.UDPAddr)) {
	mux.Handle(cmd, HandlerFunc(handler))
}

// Handle registers the handler for the given pattern.
// If a handler already exists for pattern, handle panics.
func (mux *ServeMux) Handle(cmd Cmd, handler Handler) {
	mux.mu.Lock()
	defer mux.mu.Unlock()

	if handler == nil {
		panic("urpc: nil handler")
	}
	if _, exist := mux.m[cmd]; exist {
		panic("urpc: multiple registrations for " + string(cmd))
	}

	if mux.m == nil {
		mux.m = make(map[Cmd]Handler)
	}
	mux.m[cmd] = handler
}

// ServeURPC dispatches the request to the handler whose
// cmd matches the request.
func (mux *ServeMux) ServeURPC(w PacketWriter, p Packet, addr *net.UDPAddr) {
	mux.mu.RLock()
	h, ok := mux.m[p.Cmd]
	if !ok {
		LogError("cmd not registered", p.Cmd)
		return
	}
	mux.mu.RUnlock()
	h.ServeURPC(w, p, addr)
}

// Server for urpc server
type Server struct {
	bindings []ServerBinding
	upTime   time.Time

	mu        sync.RWMutex
	listeners map[*net.UDPConn]struct{}
	done      bool

	bytesCh chan []byte
	doneCh  chan struct{}
	workCh  chan work
	wg      sync.WaitGroup // wait group for goroutines
}

// NewServer creates a server
func NewServer(bindings []ServerBinding) *Server {

	bytesChSize := numWorker() + len(bindings)
	bytesCh := make(chan []byte, bytesChSize)

	maxPacketSize := defaultMaxPacketSize
	for _, binding := range bindings {
		if binding.MaxPacketSize > maxPacketSize {
			maxPacketSize = binding.MaxPacketSize
		}
	}
	for i := 0; i < bytesChSize; i++ {
		bytesCh <- make([]byte, maxPacketSize)
	}

	return &Server{
		bindings:  bindings,
		upTime:    time.Now(),
		listeners: make(map[*net.UDPConn]struct{}),
		bytesCh:   bytesCh,
		doneCh:    make(chan struct{}),
		workCh:    make(chan work),
	}
}

// ListenAndServe starts listening on all bindings
func (srv *Server) ListenAndServe() (err error) {

	for i, binding := range srv.bindings {

		var (
			addr *net.UDPAddr
			ln   *net.UDPConn
		)
		addr, err = net.ResolveUDPAddr("udp", binding.Addr)
		if err != nil {
			srv.Shutdown()
			return
		}

		ln, err = net.ListenUDP("udp", addr)
		if err != nil {
			srv.Shutdown()
			return err
		}

		idx := i
		qrpc.GoFunc(&srv.wg, func() {
			srv.Serve(ln, idx)
		})

	}

	srv.startWorkers()
	srv.wg.Wait()
	return nil
}

const (
	defaultMaxPacketSize = 10 * 1024 * 1024
)

func numWorker() int {
	return runtime.NumCPU() + 8
}

func (srv *Server) startWorkers() {
	for i := 0; i < numWorker(); i++ {
		qrpc.GoFunc(&srv.wg, func() {
			for {
				select {
				case work := <-srv.workCh:
					packet, err := decodePacket(work.bytes[0:work.nbytes])
					if err != nil {
						LogError("decodePacket", err)
						goto return_bytes
					}
					// call handler
					srv.bindings[work.idx].Handler.ServeURPC(work.writer, packet, work.remoteAddr)

					// return bytes
				return_bytes:
					srv.bytesCh <- work.bytes
				case <-srv.doneCh:
					return
				}
			}
		})
	}
}

func (srv *Server) getBytes() (bytes []byte) {

	select {
	case bytes = <-srv.bytesCh:
		return
	case <-srv.doneCh:
		return
	}
}

var (
	errClosed = errors.New("Server closed")
)

// Serve for udp listener
func (srv *Server) Serve(ln *net.UDPConn, idx int) (err error) {

	srv.mu.Lock()
	srv.listeners[ln] = struct{}{}
	srv.mu.Unlock()

	defer func() {
		LogError("Serve done idx", idx, "err", err)
	}()

	var (
		endTime time.Time
		nbytes  int
	)

	readTimeout := srv.bindings[idx].DefaultReadTimeout
	writeTimeout := srv.bindings[idx].DefaultWriteTimeout
	writer := &packetWriter{udpConn: ln, writeTimeout: writeTimeout}

	for {
		bytes := srv.getBytes()
		if bytes == nil {
			return errClosed
		}
		if readTimeout > 0 {
			endTime = time.Now().Add(time.Duration(readTimeout) * time.Second)
			err = ln.SetReadDeadline(endTime)
			if err != nil {
				srv.mu.RLock()
				if _, ok := srv.listeners[ln]; !ok {
					srv.mu.RUnlock()
					return
				}
				srv.mu.RUnlock()

				LogError("SetReadDeadline", err)
				continue
			}
		}

		var remoteAddr *net.UDPAddr
		nbytes, remoteAddr, err = ln.ReadFromUDP(bytes)
		if err != nil {
			srv.mu.RLock()
			if _, ok := srv.listeners[ln]; !ok {
				srv.mu.RUnlock()
				return
			}
			srv.mu.RUnlock()

			LogError("ReadFromUDP", err)
			continue
		}

		srv.deliverToWorker(writer, idx, remoteAddr, bytes, nbytes)

	}

}

type work struct {
	writer     *packetWriter
	idx        int
	remoteAddr *net.UDPAddr
	bytes      []byte
	nbytes     int
}

func (srv *Server) deliverToWorker(writer *packetWriter, idx int, remoteAddr *net.UDPAddr, bytes []byte, nbytes int) {
	select {
	case srv.workCh <- work{writer: writer, idx: idx, remoteAddr: remoteAddr, bytes: bytes, nbytes: nbytes}:
	default:
		// 处理不过来就扔掉
		srv.bytesCh <- bytes
	}
}

// Shutdown the server
func (srv *Server) Shutdown() (err error) {

	srv.mu.Lock()
	if srv.done {
		srv.mu.Unlock()
		return
	}
	for ln := range srv.listeners {
		if err = ln.Close(); err != nil {
			srv.mu.Unlock()
			return
		}
		delete(srv.listeners, ln)
	}

	srv.done = true
	srv.mu.Unlock()

	close(srv.doneCh)

	srv.wg.Wait()

	return
}
