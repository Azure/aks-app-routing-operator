package main

import (
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("TestHeader", "test-header-value")
		w.Header().Set("OriginalForwardedFor", r.Header.Get("X-Forwarded-For"))
		w.Write([]byte("hello world"))
	})
	panic(http.ListenAndServe(":8080", nil))
}
