package main

import (
	"net/http"
)

func main() {
	http.HandleFunc("/healthz", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("TestHeader", "test-header-value")
		w.Header().Set("OriginalForwardedFor", r.Header.Get("X-Forwarded-For"))
		w.Write([]byte("healthz endpoint hit"))
	})
	panic(http.ListenAndServe(":8080", nil))
}
