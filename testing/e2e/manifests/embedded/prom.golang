package main

import (
	"context"
	"fmt"
	"log"
	"net/http"
	"os"
	"time"

	"github.com/prometheus/client_golang/api"
	promv1 "github.com/prometheus/client_golang/api/prometheus/v1"
)

// these need to be set in the container spec
const (
	promNsEnv   = "PROM_NS"
	promNameEnv = "PROM_NAME"
)

const (
	jobKey   = "job"
	nginxJob = "nginx-ingress"
)

func main() {
	promNs := os.Getenv(promNsEnv)
	if promNs == "" {
		panic(fmt.Errorf("missing env %s", promNsEnv))
	}
	promServer := os.Getenv(promNameEnv)
	if promServer == "" {
		panic(fmt.Errorf("missing env %s", promNameEnv))
	}

	addr := fmt.Sprintf("http://%s.%s.svc.cluster.local:9090", promServer, promNs)

	client, err := api.NewClient(api.Config{
		Address: addr,
	})
	if err != nil {
		panic(fmt.Errorf("creating prometheus client: %w", err))
	}
	api := promv1.NewAPI(client)

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
			if target.Labels[jobKey] == nginxJob && target.Health == promv1.HealthGood {
				log.Print("found healthy active nginx-ingress target")
				return
			}
		}

		log.Print("no active nginx-ingress targets found")
		w.WriteHeader(500)
	})

	panic(http.ListenAndServe(":8080", nil))
}
