package main

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"log"
	"math/rand"
	"net"
	"net/http"
	"os"
	"strings"
	"time"
)

func main() {
	nameservers := strings.Split(os.Getenv("NAMESERVERS"), ",")
	rand.Seed(time.Now().Unix())

	dialer := &net.Dialer{Resolver: &net.Resolver{
		PreferGo: true,
		Dial: func(ctx context.Context, network, address string) (net.Conn, error) {
			d := net.Dialer{Timeout: time.Second}
			var ns string
			if len(nameservers) > 1 {
				ns = nameservers[rand.Intn(len(nameservers)-1)]
				ns = ns[:len(ns)-1] // remove trailing period added for some reason by azure dns
			} else {
				ns = nameservers[0] // no need to remove trailing period if single entry coming from k8s vnet ns server
			}

			return d.DialContext(ctx, "tcp", ns+":53")
		},
	}}
	client := &http.Client{Transport: &http.Transport{
		TLSClientConfig: &tls.Config{InsecureSkipVerify: true},
		DialContext:     dialer.DialContext,
	}}

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
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
		if string(body) != "hello world" {
			log.Printf("unexpected response body: %s", body)
			w.WriteHeader(500)
			return
		}
		if val := resp.Header.Get("TestHeader"); val != "test-header-value" {
			log.Printf("unexpected test header: %s", val)
			w.WriteHeader(500)
			return
		}
		if val := resp.Header.Get("OriginalForwardedFor"); val != os.Getenv("POD_IP") {
			log.Printf("server replied with unexpected X-Forwarded-For header: %s", val)
			w.WriteHeader(500)
			return
		}
	})
	panic(http.ListenAndServe(":8080", nil))
}
