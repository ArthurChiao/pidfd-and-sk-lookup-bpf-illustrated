package main

import (
	"context"
	"flag"
	"fmt"
	"net"
	"net/http"
	"os"
	"syscall"

	"github.com/oraoto/go-pidfd"
)

var pid = flag.Int("pid", 0, "Target PID")
var fd = flag.Int("fd", 0, "Target fd")

func main() {
	flag.Parse()

	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Response from process %d\n", os.Getpid())
	})

	listenAddr := ":8080"
	if *pid != 0 && *fd != 0 {
		// Start a server, listen & serve on a socket that already exists
		dupFdAndServe(*pid, *fd, handler)
	} else {
		// Start a server, open a new socket on the given `listenAddr`
		listenAndServe(listenAddr, handler)
	}
}

// Start a normal http server
func listenAndServe(listenAddr string, handler http.HandlerFunc) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			// Print listen FD and PID for later tests
			c.Control(func(fd uintptr) { fmt.Printf("Listening on %s, fd=%d, pid=%d\n", listenAddr, fd, os.Getpid()) })
			return nil
		},
	}
	ln, err := lc.Listen(context.Background(), "tcp", listenAddr)
	panicOnError(err)

	panicOnError(http.Serve(ln, http.HandlerFunc(handler)))
}

// Start a http server by duplicating the given FD in the given process
func dupFdAndServe(targetPid int, targetFd int, handler http.HandlerFunc) {
	p, err := pidfd.Open(targetPid, 0)
	panicOnError(err)

	listenFd, err := p.GetFd(targetFd, 0)
	panicOnError(err)

	ln, err := net.FileListener(os.NewFile(uintptr(listenFd), ""))
	panicOnError(err)

	fmt.Printf("Duplicated the given socket FD and listening on it, pid=%d\n", os.Getpid())
	panicOnError(http.Serve(ln, http.HandlerFunc(handler)))
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
