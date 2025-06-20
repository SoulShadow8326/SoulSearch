package main

import (
	"io"
	"net"
	"net/http"
	"strconv"
)

type Proxy struct {
	port       int
	socketPath string
}

func CreateProxy(port int, socketPath string) *Proxy {
	return &Proxy{
		port:       port,
		socketPath: socketPath,
	}
}

func (p *Proxy) Start() {
	http.HandleFunc("/", p.proxyHandler)
	http.ListenAndServe(":"+strconv.Itoa(p.port), nil)
}

func (p *Proxy) proxyHandler(w http.ResponseWriter, r *http.Request) {
	w.Header().Set("Access-Control-Allow-Origin", "*")
	w.Header().Set("Access-Control-Allow-Methods", "GET, POST, OPTIONS")
	w.Header().Set("Access-Control-Allow-Headers", "Content-Type")

	if r.Method == "OPTIONS" {
		return
	}

	conn, err := net.Dial("unix", p.socketPath)
	if err != nil {
		http.Error(w, "Backend unavailable", http.StatusServiceUnavailable)
		return
	}
	defer conn.Close()

	req := &http.Request{
		Method:        r.Method,
		URL:           r.URL,
		Header:        r.Header,
		Body:          r.Body,
		ContentLength: r.ContentLength,
		Host:          "localhost",
	}

	err = req.Write(conn)
	if err != nil {
		http.Error(w, "Failed to forward request", http.StatusInternalServerError)
		return
	}

	io.Copy(w, conn)
}
