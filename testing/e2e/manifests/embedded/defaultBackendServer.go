package main

import (
	"net/http"
)

func main() {
	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		w.Header().Set("TestHeader", "test-header-value")
		w.Header().Set("OriginalForwardedFor", r.Header.Get("X-Forwarded-For"))
		w.WriteHeader(404)
		w.Write([]byte("404 - default backend service hit"))
	})
	panic(http.ListenAndServe(":8080", nil))
}
