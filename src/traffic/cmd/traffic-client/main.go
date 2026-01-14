package main

import (
	"crypto/rand"
	"flag"
	"fmt"
	"log"
	"net"
	"time"
)

type profile struct {
	name     string
	proto    string
	port     int
	size     int
	interval time.Duration
}

func main() {
	target := flag.String("target", "127.0.0.1", "server address")
	mode := flag.String("mode", "voice", "voice|video|email|web")
	duration := flag.Duration("duration", 20*time.Second, "run duration")
	level := flag.Int("level", 3, "intensity level 1-5")
	flag.Parse()

	log.SetPrefix("[traffic-client] ")

	p := resolveProfile(*mode, *level)
	log.Printf("target=%s mode=%s proto=%s port=%d size=%d interval=%s", *target, p.name, p.proto, p.port, p.size, p.interval)

	deadline := time.Now().Add(*duration)
	payload := make([]byte, p.size)
	_, _ = rand.Read(payload)

	if p.proto == "udp" {
		sendUDP(*target, p.port, payload, p.interval, deadline)
		return
	}
	sendTCP(*target, p.port, payload, p.interval, deadline)
}

func resolveProfile(mode string, level int) profile {
	if level < 1 {
		level = 1
	}
	if level > 5 {
		level = 5
	}
	switch mode {
	case "video":
		return profile{name: "video", proto: "udp", port: 5006, size: 1200, interval: scaleInterval(10*time.Millisecond, level)}
	case "email":
		return profile{name: "email", proto: "tcp", port: 2525, size: 12 * 1024, interval: scaleInterval(2*time.Second, level)}
	case "web":
		return profile{name: "web", proto: "tcp", port: 8081, size: 48 * 1024, interval: scaleInterval(300*time.Millisecond, level)}
	default:
		return profile{name: "voice", proto: "udp", port: 5004, size: 160, interval: scaleInterval(20*time.Millisecond, level)}
	}
}

func scaleInterval(base time.Duration, level int) time.Duration {
	return time.Duration(float64(base) / (0.5 + float64(level)/2.5))
}

func sendUDP(host string, port int, payload []byte, interval time.Duration, deadline time.Time) {
	addr := fmt.Sprintf("%s:%d", host, port)
	conn, err := net.Dial("udp", addr)
	if err != nil {
		log.Fatalf("udp dial failed: %v", err)
	}
	defer conn.Close()
	ticker := time.NewTicker(interval)
	defer ticker.Stop()
	for time.Now().Before(deadline) {
		_, _ = conn.Write(payload)
		<-ticker.C
	}
}

func sendTCP(host string, port int, payload []byte, interval time.Duration, deadline time.Time) {
	addr := fmt.Sprintf("%s:%d", host, port)
	for time.Now().Before(deadline) {
		conn, err := net.Dial("tcp", addr)
		if err == nil {
			_, _ = conn.Write(payload)
			conn.Close()
		}
		time.Sleep(interval)
	}
}
