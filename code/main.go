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
	mode        string
	port        uint
	socketPath  string
	dialTimeout time.Duration
}

func (c config) validate() error {
	if c.mode != "listen" && c.mode != "connect" {
		return errors.New("mode must be listen or connect")
	}
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

func proxyConnections(left, right net.Conn) {
	defer left.Close()
	defer right.Close()
	done := make(chan struct{}, 2)
	copyStream := func(dst, src net.Conn) {
		_, _ = io.Copy(dst, src)
		done <- struct{}{}
	}
	go copyStream(left, right)
	go copyStream(right, left)
	<-done
}

func runListen(cfg config) error {
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
		go func(host net.Conn) {
			plugin, err := dialUnix(cfg.socketPath, cfg.dialTimeout)
			if err != nil {
				host.Close()
				log.Printf("plugin socket unavailable: %v", err)
				return
			}
			proxyConnections(host, plugin)
		}(conn)
	}
}

func runConnect(cfg config) error {
	if err := os.MkdirAll(filepath.Dir(cfg.socketPath), 0755); err != nil {
		return fmt.Errorf("create Unix socket directory: %w", err)
	}
	if err := os.Remove(cfg.socketPath); err != nil && !errors.Is(err, os.ErrNotExist) {
		return fmt.Errorf("remove stale Unix socket: %w", err)
	}
	listener, err := net.Listen("unix", cfg.socketPath)
	if err != nil {
		return fmt.Errorf("listen on Unix socket %s: %w", cfg.socketPath, err)
	}
	defer listener.Close()
	defer os.Remove(cfg.socketPath)
	if err := os.Chmod(cfg.socketPath, 0660); err != nil {
		return fmt.Errorf("set Unix socket permissions: %w", err)
	}

	log.Printf("forwarding unix socket %s to host vsock port %d", cfg.socketPath, cfg.port)
	for {
		conn, err := listener.Accept()
		if err != nil {
			return fmt.Errorf("accept Unix socket connection: %w", err)
		}
		go func(guest net.Conn) {
			host, err := vsock.Dial(vsock.Host, uint32(cfg.port), nil)
			if err != nil {
				guest.Close()
				log.Printf("host vsock unavailable: %v", err)
				return
			}
			proxyConnections(guest, host)
		}(conn)
	}
}

func run(cfg config) error {
	if err := cfg.validate(); err != nil {
		return err
	}
	if cfg.mode == "connect" {
		return runConnect(cfg)
	}
	return runListen(cfg)
}

func main() {
	log.SetFlags(0)
	var cfg config
	flag.StringVar(&cfg.mode, "mode", "listen", "vsock direction: listen or connect")
	flag.UintVar(&cfg.port, "port", 4040, "guest vsock port")
	flag.StringVar(&cfg.socketPath, "socket", "", "guest-local plugin Unix socket")
	flag.DurationVar(&cfg.dialTimeout, "dial-timeout", 10*time.Second, "Unix socket startup wait")
	flag.Parse()

	if err := run(cfg); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
