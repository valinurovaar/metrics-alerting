package main

import (
	"flag"
	"log"
	"os"
	"strconv"
	"time"

	"metrics-alerting/internal/agent"
)

func main() {
	addr := flag.String("a", "localhost:8080", "HTTP server address")
	reportInterval := flag.Int("r", 10, "Report interval in seconds")
	pollInterval := flag.Int("p", 2, "Poll interval in seconds")

	flag.Parse()

	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		*addr = envAddr
	}

	if envReport := os.Getenv("REPORT_INTERVAL"); envReport != "" {
		if value, err := strconv.Atoi(envReport); err == nil {
			*reportInterval = value
		}
	}

	if envPoll := os.Getenv("POLL_INTERVAL"); envPoll != "" {
		if value, err := strconv.Atoi(envPoll); err == nil {
			*pollInterval = value
		}
	}

	a := agent.New(*addr)

	a.SetReportInterval(time.Duration(*reportInterval) * time.Second)
	a.SetPollInterval(time.Duration(*pollInterval) * time.Second)

	log.Printf(
		"Starting metrics agent, server: %s, report interval: %ds, poll interval: %ds",
		*addr,
		*reportInterval,
		*pollInterval,
	)

	a.Run()
}