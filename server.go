package ccafct

import (
	"context"
	"fmt"
	"log"
	"net"
	"net/http"
	"os"
	"strconv"

	"golang.org/x/sys/unix"
)

// DefaultListenAddr is the default listen address.
var DefaultListenAddr = fmt.Sprintf(":%d", DefaultPort)

// DefaultBufLen is the default buffer length.
const DefaultBufLen = 32 * 1024

// ctxKey is a custom type for context keys.
type ctxKey string

// connCtxKey is the net.Conn context key.
const connCtxKey = ctxKey("net.Conn")

// Server is the FCT server.
type Server struct {
	// ListenAddr is the server listen address (default :8188)
	ListenAddr string

	// BufLen is the length of the buffer for writing responses.
	BufLen int

	buf []byte
}

// init initializes the Server struct.
func (s *Server) init() {
	if s.ListenAddr == "" {
		s.ListenAddr = DefaultListenAddr
	}

	if s.BufLen == 0 {
		s.BufLen = DefaultBufLen
	}

	s.buf = make([]byte, s.BufLen)
}

// Run runs the server.
func (s *Server) Run() error {
	s.init()

	http.HandleFunc(FCTPath, s.handleFCT)

	server := http.Server{
		Addr: s.ListenAddr,
		ConnContext: func(ctx context.Context, c net.Conn) context.Context {
			return context.WithValue(ctx, connCtxKey, c)
		},
	}

	log.Printf("server listening on %s", s.ListenAddr)

	return server.ListenAndServe()
}

// handleFCT is the HandleFunc for the FCT request path.
func (s *Server) handleFCT(w http.ResponseWriter, r *http.Request) {
	var flen int64
	var fstr string
	var err error

	if err = s.setSockOpts(r); err != nil {
		http.Error(w, err.Error(), http.StatusBadRequest)
		return
	}

	if fstr = r.Header.Get(FlowLengthHeader); fstr == "" {
		http.Error(w, fmt.Sprintf("missing '%s' header", FlowLengthHeader),
			http.StatusBadRequest)
		return
	}

	if flen, err = strconv.ParseInt(fstr, 10, 64); err != nil || flen < 0 {
		http.Error(w,
			fmt.Sprintf("invalid %s: '%s'", FlowLengthHeader, fstr),
			http.StatusBadRequest)
		return
	}

	var n int
	for r := flen; r > 0; r -= int64(n) {
		l := s.BufLen
		if r < int64(s.BufLen) {
			l = int(r)
		}
		if n, err = w.Write(s.buf[:l]); err != nil {
			log.Printf("write error: '%s'", err.Error())
			break
		}
	}
}

// setSockOpts sets socket options for the request. Detailed error information
// is logged, while the error returned is suitable for sending to the client.
func (s *Server) setSockOpts(r *http.Request) (err error) {
	var tcpConn *net.TCPConn
	var ok bool
	var f *os.File
	var cca string

	if cca = r.Header.Get(CCAHeader); cca == "" {
		return
	}

	conn := r.Context().Value(connCtxKey)
	if tcpConn, ok = conn.(*net.TCPConn); !ok {
		log.Printf("setSockOpts request not tcpConn: '%v'", conn)
		err = ccaError(cca)
		return
	}
	if f, err = tcpConn.File(); err != nil {
		log.Printf("setSockOpts unable to get file: %s", err)
		err = ccaError(cca)
		return
	}

	fd := int(f.Fd())

	if err = unix.SetsockoptString(fd, unix.IPPROTO_TCP,
		unix.TCP_CONGESTION, cca); err != nil {
		log.Printf("setSockOpts unable to set TCP_CONGESTION to '%s': '%s'",
			cca, err)
		err = ccaError(cca)
		return
	}
	/*
		if cca, err = unix.GetsockoptString(fd, unix.IPPROTO_TCP,
			unix.TCP_CONGESTION); err != nil {
			log.Printf("setSockOpts unable to get TCP_CONGESTION: '%s'", err)
			return
		}
	*/

	return
}

func ccaError(cca string) error {
	return fmt.Errorf("unable to use selected CCA: '%s'", cca)
}
