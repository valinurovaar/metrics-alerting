package main

import (
	"flag"
	"log"
	"time"

	"metrics-alerting/internal/agent"
)

func main() {
	addr := flag.String("a", "localhost:8080", "HTTP server address")
	reportInterval := flag.Int("r", 10, "Report interval in seconds")
	pollInterval := flag.Int("p", 2, "Poll interval in seconds")
	flag.Parse()

	a := agent.New(*addr)
	a.SetReportInterval(time.Duration(*reportInterval) * time.Second)
	a.SetPollInterval(time.Duration(*pollInterval) * time.Second)
	log.Printf("Starting metrics agent, server: %s, report interval: %ds, poll interval: %ds", *addr, *reportInterval, *pollInterval)
	a.Run()
}
