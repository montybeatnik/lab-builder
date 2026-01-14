package main

import (
	"flag"
	"fmt"
	"io"
	"log"
	"net"
	"os"
	"os/signal"
	"sync/atomic"
	"time"
)

type counter struct {
	bytes int64
}

func (c *counter) add(n int64) {
	atomic.AddInt64(&c.bytes, n)
}

func (c *counter) snapshot() int64 {
	return atomic.LoadInt64(&c.bytes)
}

func main() {
	var tcpPort int
	var udpPort int
	var reportEvery time.Duration
	flag.IntVar(&tcpPort, "tcp", 8081, "tcp listen port")
	flag.IntVar(&udpPort, "udp", 5004, "udp listen port")
	flag.DurationVar(&reportEvery, "report", 5*time.Second, "report interval")
	flag.Parse()

	log.SetPrefix("[traffic-server] ")
	log.Printf("listening tcp=%d udp=%d", tcpPort, udpPort)

	tcpCounter := &counter{}
	udpCounter := &counter{}

	go listenTCP(tcpPort, tcpCounter)
	go listenUDP(udpPort, udpCounter)

	ticker := time.NewTicker(reportEvery)
	defer ticker.Stop()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, os.Interrupt)

	for {
		select {
		case <-ticker.C:
			log.Printf("tcp bytes=%d udp bytes=%d", tcpCounter.snapshot(), udpCounter.snapshot())
		case <-stop:
			log.Println("shutting down")
			return
		}
	}
}

func listenTCP(port int, c *counter) {
	ln, err := net.Listen("tcp", fmt.Sprintf(":%d", port))
	if err != nil {
		log.Fatalf("tcp listen failed: %v", err)
	}
	defer ln.Close()

	for {
		conn, err := ln.Accept()
		if err != nil {
			log.Printf("tcp accept error: %v", err)
			continue
		}
		go func() {
			defer conn.Close()
			buf := make([]byte, 32*1024)
			for {
				n, err := conn.Read(buf)
				if n > 0 {
					c.add(int64(n))
				}
				if err != nil {
					if err != io.EOF {
						log.Printf("tcp read error: %v", err)
					}
					return
				}
			}
		}()
	}
}

func listenUDP(port int, c *counter) {
	addr := net.UDPAddr{Port: port}
	conn, err := net.ListenUDP("udp", &addr)
	if err != nil {
		log.Fatalf("udp listen failed: %v", err)
	}
	defer conn.Close()

	buf := make([]byte, 64*1024)
	for {
		n, _, err := conn.ReadFromUDP(buf)
		if n > 0 {
			c.add(int64(n))
		}
		if err != nil {
			log.Printf("udp read error: %v", err)
		}
	}
}
