package main

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"time"
)

func main() {
	nameserver := os.Getenv("NAMESERVER")

	dialer := &net.Dialer{Resolver: &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: time.Second}
			var ns string
			if nameserver[:len(nameserver)-1] == "." {
				ns = nameserver[:len(ns)-1] // remove trailing period added for some reason by azure dns
			} else {
				ns = nameserver // no need to remove trailing period if single entry coming from k8s vnet ns server
			}

			return d.DialContext(ctx, "tcp", ns+":53")
		},
	}}
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext:     dialer.DialContext,
	}}

	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		resp, err := client.Get(os.Getenv("URL"))
		if err != nil {
			log.Printf("error sending request: %s", err)
			w.WriteHeader(500)
			return
		}
		defer resp.Body.Close()

		body, err := ioutil.ReadAll(resp.Body)
		if err != nil {
			log.Printf("error reading response body: %s", err)
			w.WriteHeader(500)
			return
		}
		log.Printf("received response %s from url %s", string(body), os.Getenv("URL"))
		if string(body) != "healthz endpoint hit" {
			log.Printf("unexpected response body: %s", body)
			w.WriteHeader(500)
			return
		}
		if val := resp.Header.Get("TestHeader"); val != "test-header-value" {
			log.Printf("unexpected test header: %s", val)
			w.WriteHeader(500)
			return
		}

		expectedIp := os.Getenv("POD_IP")
		if val := resp.Header.Get("OriginalForwardedFor"); val != expectedIp {
			log.Printf("server replied with unexpected X-Forwarded-For header: %s, expected %s", val, expectedIp)
			w.WriteHeader(500)
			return
		}
	})
	panic(http.ListenAndServe(":8080", nil))
}
