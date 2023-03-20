package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	v1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// these need to be the same as the consts in ../prometheus.go
const (
	promNsEnv  = "PROM_NS"
	promServer = "prometheus-server"
)

const (
	jobKey   = "job"
	nginxJob = "nginx-ingress"
)

func main() {
	promNs := os.Getenv(promNsEnv)
	addr := fmt.Sprintf("http://%s.%s.svc.cluster.local:9090", promServer, promNs)

	client, err := api.NewClient(api.Config{
		Address: addr,
	})
	if err != nil {
		panic(fmt.Errorf("creating prometheus client: %w", err))
	}
	api := v1.NewAPI(client)

	http.HandleFunc("/", func(w http.ResponseWriter, r *http.Request) {
		ctx, cancel := context.WithTimeout(context.Background(), 10*time.Second)
		defer cancel()

		targets, err := api.Targets(ctx)
		if err != nil {
			log.Printf("error listing prometheus targets: %s", err)
			w.WriteHeader(500)
			return
		}

		for _, target := range targets.Active {
			if target.Labels[jobKey] == nginxJob {
				log.Print("found active nginx-ingress target")
				return
			}
		}

		log.Print("no active nginx-ingress targets found")
		w.WriteHeader(500)

	})

	panic(http.ListenAndServe(":8080", nil))
}
