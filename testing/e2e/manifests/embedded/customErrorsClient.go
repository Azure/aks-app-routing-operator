package main

import (
	"context"
	"crypto/tls"
	"io/ioutil"
	"log"
	"net"
	"net/http"
	"os"
	"regexp"
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

	liveServiceBody := "live service"
	deadServiceBody := "<body>CONFIRMING CUSTOM 503 TEST MESSAGE</body>"
	notFoundBody := "<body>CONFIRMING CUSTOM 404 TEST MESSAGE</body>"

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

		cleanedBody := cleanBody(string(body)[:(len(liveServiceBody))])
		log.Printf("received response: %s", cleanedBody)
		if cleanedBody != liveServiceBody {
			log.Printf("unexpected response body: %s length: %d, expected: %s length: %d", cleanedBody, len(string(body)), liveServiceBody, len(liveServiceBody))
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

		cleanedBody = cleanBody(string(body))
		log.Printf("received response %s from url %s", cleanedBody, os.Getenv("TEST_URL"))
		if cleanedBody != deadServiceBody {
			log.Printf("unexpected response body: %s expected: %s", cleanedBody, deadServiceBody)
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

		cleanedBody = cleanBody(string(body))
		log.Printf("received response %s from url %s", cleanedBody, os.Getenv("TEST_URL"))
		if cleanedBody != notFoundBody {
			log.Printf("unexpected response body: %s expected: %s", cleanedBody, notFoundBody)
			w.WriteHeader(500)
			return
		}
	})
	panic(http.ListenAndServe(":8080", nil))
}

func cleanBody(body string) string {
	pattern := "/(?:\\r\\n|\\r|\\n)/g"
	cleaner := regexp.MustCompile(pattern)

	return cleaner.ReplaceAllString(body, "")
}
