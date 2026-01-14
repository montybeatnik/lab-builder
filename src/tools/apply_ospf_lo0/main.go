package main

import (
	"bytes"
	"crypto/tls"
	"encoding/json"
	"flag"
	"fmt"
	"io"
	"net/http"
	"os"
	"regexp"
	"strings"
	"sync"
	"time"
)

type rpcResp struct {
	Result []map[string]any `json:"result"`
	Error  *struct {
		Code    int    `json:"code"`
		Message string `json:"message"`
	} `json:"error"`
}

func main() {
	var (
		user        = flag.String("user", "admin", "eAPI username")
		pass        = flag.String("pass", "admin", "eAPI password")
		ips         = flag.String("ips", "", "comma-separated IPs")
		ipFile      = flag.String("ip-file", "", "file with one IP per line")
		areaCmd     = flag.String("area-cmd", "show running-config section ospf", "command to discover OSPF area")
		areaDefault = flag.String("area-default", "0.0.0.0", "default OSPF area when discovery fails")
		timeout     = flag.Duration("timeout", 6*time.Second, "per-device timeout")
		workers     = flag.Int("workers", 8, "max concurrent requests")
	)
	flag.Parse()

	targets, err := loadIPs(*ips, *ipFile)
	if err != nil {
		fmt.Fprintln(os.Stderr, "error:", err)
		os.Exit(1)
	}
	if len(targets) == 0 {
		fmt.Fprintln(os.Stderr, "no targets provided")
		os.Exit(1)
	}

	sem := make(chan struct{}, *workers)
	var wg sync.WaitGroup
	for _, ip := range targets {
		wg.Add(1)
		go func(ip string) {
			defer wg.Done()
			sem <- struct{}{}
			defer func() { <-sem }()

			ctx := &http.Client{
				Transport: &http.Transport{
					TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
				},
				Timeout: *timeout,
			}

			area, err := discoverArea(ctx, ip, *user, *pass, *areaCmd)
			if err != nil {
				area = *areaDefault
				fmt.Printf("%s area lookup failed: %v (using %s)\n", ip, err, area)
			}

			cmds := []string{
				"enable",
				"configure",
				"interface Loopback0",
				"ip ospf area " + area,
			}
			if err := runCmds(ctx, ip, *user, *pass, cmds); err != nil {
				fmt.Printf("%s apply failed: %v\n", ip, err)
				return
			}
			fmt.Printf("%s applied ospf area %s on Loopback0\n", ip, area)
		}(ip)
	}
	wg.Wait()
}

func loadIPs(list, file string) ([]string, error) {
	var out []string
	if list != "" {
		for _, item := range strings.Split(list, ",") {
			item = strings.TrimSpace(item)
			if item != "" {
				out = append(out, item)
			}
		}
	}
	if file == "" {
		return out, nil
	}
	b, err := os.ReadFile(file)
	if err != nil {
		return nil, err
	}
	for _, line := range strings.Split(string(b), "\n") {
		line = strings.TrimSpace(line)
		if line != "" {
			out = append(out, line)
		}
	}
	return out, nil
}

func discoverArea(client *http.Client, ip, user, pass, cmd string) (string, error) {
	body, err := runCmdsRaw(client, ip, user, pass, []string{cmd}, "text")
	if err != nil {
		return "", err
	}
	var resp rpcResp
	if err := json.Unmarshal(body, &resp); err != nil {
		return "", err
	}
	if resp.Error != nil {
		return "", fmt.Errorf("eAPI error: %s", resp.Error.Message)
	}
	if len(resp.Result) == 0 {
		return "", fmt.Errorf("empty eAPI response")
	}
	text, _ := resp.Result[0]["output"].(string)
	area := parseArea(text)
	if area == "" {
		return "", fmt.Errorf("could not parse area")
	}
	return area, nil
}

func parseArea(output string) string {
	re := regexp.MustCompile(`(?i)area\s+([0-9.]+)`)
	m := re.FindStringSubmatch(output)
	if len(m) < 2 {
		return ""
	}
	return m[1]
}

func runCmds(client *http.Client, ip, user, pass string, cmds []string) error {
	_, err := runCmdsRaw(client, ip, user, pass, cmds, "json")
	return err
}

func runCmdsRaw(client *http.Client, ip, user, pass string, cmds []string, format string) ([]byte, error) {
	if format == "" {
		format = "json"
	}
	payload := map[string]any{
		"jsonrpc": "2.0",
		"method":  "runCmds",
		"params": map[string]any{
			"version": 1,
			"format":  format,
			"cmds":    cmds,
		},
		"id": 1,
	}
	b, _ := json.Marshal(payload)

	req, err := http.NewRequest(http.MethodPost, "https://"+ip+"/command-api", bytes.NewReader(b))
	if err != nil {
		return nil, err
	}
	req.Header.Set("Content-Type", "application/json")
	req.SetBasicAuth(user, pass)

	resp, err := client.Do(req)
	if err != nil {
		return nil, err
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil, err
	}
	if resp.StatusCode < 200 || resp.StatusCode >= 300 {
		return nil, fmt.Errorf("status %d: %s", resp.StatusCode, string(body))
	}
	return body, nil
}
