package main

import (
	"context"
	"fmt"
	"net"
	"net/http"
	"os"
	"os/exec"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"github.com/oraoto/go-pidfd"
)

type Server struct {
	listenFd int
	listener net.Listener
	http.Server
}

func info(msg string) {
	ts := time.Now().Format("2006-01-02T15:04:05")
	pid := os.Getpid()
	fmt.Printf("[%s PID=%d] %s\n", ts, pid, msg)
}

func main() {
	s := &Server{}

	go s.handleUpgradeSignal()  // SIGHUP
	go s.handleShutdownSignal() // SIGTERM, SIGINT

	// Get target PID and target FD from env
	pid, _ := strconv.Atoi(os.Getenv("_PID"))
	fd, _ := strconv.Atoi(os.Getenv("_FD"))
	handler := http.HandlerFunc(func(w http.ResponseWriter, r *http.Request) {
		fmt.Fprintf(w, "Response from process %d\n", os.Getpid())
	})

	if pid > 0 && fd > 0 {
		s.dupFdAndServe(pid, fd, handler)
	} else {
		s.listenAndServe(":8080", handler)
	}
}

func (s *Server) listenAndServe(addr string, handler http.Handler) {
	lc := net.ListenConfig{
		Control: func(network, address string, c syscall.RawConn) error {
			c.Control(func(fd uintptr) {
				s.listenFd = int(fd)
			})
			return nil
		},
	}
	ln, err := lc.Listen(context.Background(), "tcp", addr)
	panicOnError(err)
	s.listener = ln

	s.Addr = addr
	s.Handler = handler

	info(fmt.Sprintf("server up, listening on %s", addr))
	s.Serve(s.listener)
}

func (s *Server) dupFdAndServe(pid int, fd int, handler http.Handler) {
	p, err := pidfd.Open(pid, 0)
	panicOnError(err)

	listenFd, err := p.GetFd(fd, 0)
	panicOnError(err)

	s.listenFd = listenFd

	ln, err := net.FileListener(os.NewFile(uintptr(listenFd), ""))
	panicOnError(err)

	var errChan = make(chan error)
	go func() {
		info("duplicated the given socket FD and listening on it")
		errChan <- http.Serve(ln, handler)
	}()

	select {
	case err := <-errChan:
		panicOnError(err)
	case <-time.After(time.Second * 5):
		info("5 seconds have past since new instance up, going to stop the old server")
		p.SendSignal(syscall.SIGTERM, 0)
	}

	err = <-errChan
	panicOnError(err)
}

func (s *Server) handleUpgradeSignal() {
	upgradeCh := make(chan os.Signal, 1)
	signal.Notify(upgradeCh, syscall.SIGHUP)

	for {
		<-upgradeCh
		info("received SIGUP, going to upgrade")

		// Pass pid and listenfd to child process
		env := os.Environ()
		env = append(env, fmt.Sprintf("_PID=%d", os.Getpid()))
		env = append(env, fmt.Sprintf("_FD=%d", s.listenFd))

		cmd := exec.Command(os.Args[0], os.Args[1:]...)
		cmd.Env = env
		cmd.Stdout = os.Stdout // inherit stdout/stderr so we can see logs in the same window
		cmd.Stderr = os.Stderr

		if err := cmd.Start(); err != nil {
			fmt.Println(err)
			cmd.Wait()
		} else {
			info("start new instance (process) done, serving on the same socket")
		}
	}
}

func (s *Server) handleShutdownSignal() {
	shutdownCh := make(chan os.Signal, 1)
	signal.Notify(shutdownCh, syscall.SIGTERM, syscall.SIGINT)

	<-shutdownCh
	info("received SIGTERM/SIGINT, going to shutdown")

	ctx, cancel := context.WithTimeout(context.Background(), 5*time.Second)
	defer cancel()
	if err := s.Shutdown(ctx); err != nil {
		info(fmt.Sprintf("fail to shutdown: %s", err))
	}
}

func panicOnError(err error) {
	if err != nil {
		panic(err)
	}
}
