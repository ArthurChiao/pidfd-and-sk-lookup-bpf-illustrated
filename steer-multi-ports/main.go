//go:build linux
// +build linux

package main

import (
	"log"
	"os"
	"path/filepath"
	"time"

	"github.com/cilium/ebpf/link"
	"github.com/cilium/ebpf/rlimit"
	pidfd "github.com/oraoto/go-pidfd"
)

// $BPF_CLANG and $BPF_CFLAGS are set by the Makefile.
//go:generate go run github.com/cilium/ebpf/cmd/bpf2go -cc $BPF_CLANG -cflags $BPF_CFLAGS bpf sk_lookup.c -- -I../headers

func main() {
	// Allow the current process to lock memory for eBPF resources.
	if err := rlimit.RemoveMemlock(); err != nil {
		log.Fatal(err)
	}

	log.Println("Loading BPF objects ...")

	// Load pre-compiled programs and maps into the kernel.
	objs := bpfObjects{}
	if err := loadBpfObjects(&objs, nil); err != nil {
		log.Fatalf("loading objects: %v", err)
	}
	defer objs.Close()

	log.Println("Opening netns ...")

	// This can be a path to another netns as well.
	netns, err := os.Open("/proc/self/ns/net")
	if err != nil {
		panic(err)
	}
	defer netns.Close()

	log.Println("Linking BPF prog to netns ...")

	prog := objs.EchoDispatch
	link, err := link.AttachNetNs(int(netns.Fd()), prog)
	if err != nil {
		panic(err)
	}

	log.Println("Pinning BPF link to bpffs ...")

	path := filepath.Join("/sys/fs/bpf/", "echo_dispatch_link")
	os.Remove(path)
	// if err = link.Pin(path); err != nil {
	// 	panic(err)
	// }

	log.Println("Duplicating socket FD ...")

	targetPid := 127706
	targetPidFd, err := pidfd.Open(targetPid, 0)
	if err != nil {
		panic(err)
	}

	log.Println("Target PidFd ... ", targetPidFd)

	targetFd := 3
	sockFd, err := targetPidFd.GetFd(targetFd, 0)
	if err != nil {
		panic(err)
	}

	log.Println("Storing duplicated sockFD into sockmap ... ", sockFd)

	var key uint32 = 0
	var val uint64 = uint64(sockFd)
	if err := objs.SocketMap.Put(&key, &val); err != nil {
		panic(err)
	}

	log.Println("Sleeping for some time ...")
	time.Sleep(6000 * time.Second)

	// The socket lookup program is now active until Close().
	link.Close()
}
