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

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		// live service tests
		liveResp, err := client.Get(os.Getenv("LIVE"))
		if err != nil {
			log.Printf("error sending request: %s", err)
			w.WriteHeader(500)
			return
		}
		defer liveResp.Body.Close()

		body, err := ioutil.ReadAll(liveResp.Body)
		if err != nil {
			log.Printf("error reading response body: %s", err)
			w.WriteHeader(500)
			return
		}
		log.Printf("received response: %s", string(body))
		if string(body) != "live service" {
			log.Printf("unexpected response body: %s", string(body))
			w.WriteHeader(500)
			return
		}
		if val := liveResp.Header.Get("TestHeader"); val != "test-header-value" {
			log.Printf("unexpected test header: %s", val)
			w.WriteHeader(500)
			return
		}

		expectedIp := os.Getenv("POD_IP")
		if val := liveResp.Header.Get("OriginalForwardedFor"); val != expectedIp {
			log.Printf("server replied with unexpected X-Forwarded-For header: %s, expected %s", val, expectedIp)
			w.WriteHeader(500)
			return
		}

		// dead service tests - 503 error codes
		deadResp, err := client.Get(os.Getenv("DEAD"))
		if err != nil {
			log.Printf("error sending request: %s", err)
			w.WriteHeader(500)
			return
		}
		defer deadResp.Body.Close()

		body, err = ioutil.ReadAll(deadResp.Body)
		if err != nil {
			log.Printf("error reading response body: %s", err)
			w.WriteHeader(500)
			return
		}
		log.Printf("received response %s from url %s", string(body), os.Getenv("TEST_URL"))
		if string(body) != "503 CUSTOM TEST MESSAGE" {
			log.Printf("unexpected response body: %s", body)
			w.WriteHeader(500)
			return
		}
		if val := deadResp.Header.Get("TestHeader"); val != "test-header-value" {
			log.Printf("unexpected test header: %s", val)
			w.WriteHeader(500)
			return
		}

		expectedIp = os.Getenv("POD_IP")
		if val := deadResp.Header.Get("OriginalForwardedFor"); val != expectedIp {
			log.Printf("server replied with unexpected X-Forwarded-For header: %s, expected %s", val, expectedIp)
			w.WriteHeader(500)
			return
		}

		// not found tests - 404 error codes
		notFoundResp, err := client.Get(os.Getenv("NOT_FOUND"))
		if err != nil {
			log.Printf("error sending request: %s", err)
			w.WriteHeader(500)
			return
		}
		defer notFoundResp.Body.Close()

		body, err = ioutil.ReadAll(notFoundResp.Body)
		if err != nil {
			log.Printf("error reading response body: %s", err)
			w.WriteHeader(500)
			return
		}
		log.Printf("received response %s from url %s", string(body), os.Getenv("TEST_URL"))
		if string(body) != "503 CUSTOM TEST MESSAGE" {
			log.Printf("unexpected response body: %s", body)
			w.WriteHeader(500)
			return
		}
		if val := notFoundResp.Header.Get("TestHeader"); val != "test-header-value" {
			log.Printf("unexpected test header: %s", val)
			w.WriteHeader(500)
			return
		}

		expectedIp = os.Getenv("POD_IP")
		if val := notFoundResp.Header.Get("OriginalForwardedFor"); val != expectedIp {
			log.Printf("server replied with unexpected X-Forwarded-For header: %s, expected %s", val, expectedIp)
			w.WriteHeader(500)
			return
		}
	})
	panic(http.ListenAndServe(":8080", nil))
}
