package main

import (
	"errors"
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"path/filepath"
	"time"

	"github.com/mdlayher/vsock"
)

type config struct {
	port        uint
	socketPath  string
	dialTimeout time.Duration
}

func (c config) validate() error {
	if c.port == 0 || uint64(c.port) > uint64(^uint32(0)) {
		return errors.New("vsock port must be between 1 and 4294967295")
	}
	if !filepath.IsAbs(c.socketPath) {
		return errors.New("plugin socket must be an absolute path")
	}
	if c.dialTimeout <= 0 {
		return errors.New("dial timeout must be positive")
	}
	return nil
}

func dialUnix(path string, timeout time.Duration) (net.Conn, error) {
	deadline := time.Now().Add(timeout)
	var lastErr error
	for {
		conn, err := net.DialTimeout("unix", path, 500*time.Millisecond)
		if err == nil {
			return conn, nil
		}
		lastErr = err
		if time.Now().After(deadline) {
			return nil, lastErr
		}
		time.Sleep(50 * time.Millisecond)
	}
}

func proxyConnection(host net.Conn, cfg config) {
	defer host.Close()

	plugin, err := dialUnix(cfg.socketPath, cfg.dialTimeout)
	if err != nil {
		log.Printf("plugin socket unavailable: %v", err)
		return
	}
	defer plugin.Close()

	done := make(chan struct{}, 2)
	copyStream := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go copyStream(plugin, host)
	go copyStream(host, plugin)
	<-done
}

func run(cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}

	listener, err := vsock.ListenContextID(^uint32(0), uint32(cfg.port), nil)
	if err != nil {
		return fmt.Errorf("listen on guest vsock port %d: %w", cfg.port, err)
	}
	defer listener.Close()

	log.Printf("forwarding guest vsock port %d to unix socket %s", cfg.port, cfg.socketPath)
	for {
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("accept vsock connection: %w", err)
		}
		go proxyConnection(conn, cfg)
	}
}

func main() {
	log.SetFlags(0)
	var cfg config
	flag.UintVar(&cfg.port, "port", 4040, "guest vsock port")
	flag.StringVar(&cfg.socketPath, "socket", "", "guest-local plugin Unix socket")
	flag.DurationVar(&cfg.dialTimeout, "dial-timeout", 10*time.Second, "Unix socket startup wait")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
