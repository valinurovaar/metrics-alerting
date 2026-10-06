package main

import (
	"context"
	"flag"
	"log"
	"os"
	"os/signal"
	"strconv"
	"syscall"
	"time"

	"metrics-alerting/internal/agent"
)

func main() {
	addr := flag.String("a", "localhost:8080", "HTTP server address")
	reportInterval := flag.Int("r", 10, "Report interval in seconds")
	pollInterval := flag.Int("p", 2, "Poll interval in seconds")
	key := flag.String("k", "", "Key for signing request bodies")
	rateLimit := flag.Int("l", agent.RateLimit, "Maximum number of concurrent outgoing requests")

	flag.Parse()

	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		*addr = envAddr
	}

	if envReport := os.Getenv("REPORT_INTERVAL"); envReport != "" {
		if value, err := strconv.Atoi(envReport); err == nil {
			*reportInterval = value
		} else {
			log.Printf("Warning: invalid REPORT_INTERVAL %q, using current value %d", envReport, *reportInterval)
		}
	}

	if envPoll := os.Getenv("POLL_INTERVAL"); envPoll != "" {
		if value, err := strconv.Atoi(envPoll); err == nil {
			*pollInterval = value
		} else {
			log.Printf("Warning: invalid POLL_INTERVAL %q, using current value %d", envPoll, *pollInterval)
		}
	}
	if envKey := os.Getenv("KEY"); envKey != "" {
		*key = envKey
	}
	if envRateLimit := os.Getenv("RATE_LIMIT"); envRateLimit != "" {
		if value, err := strconv.Atoi(envRateLimit); err == nil && value > 0 {
			*rateLimit = value
		} else {
			log.Printf("Warning: invalid RATE_LIMIT %q, using current value %d", envRateLimit, *rateLimit)
		}
	}

	a := agent.New(*addr)
	a.SetKey(*key)

	a.SetReportInterval(time.Duration(*reportInterval) * time.Second)
	a.SetPollInterval(time.Duration(*pollInterval) * time.Second)
	a.SetRateLimit(*rateLimit)

	log.Printf(
		"Starting metrics agent, server: %s, report interval: %ds, poll interval: %ds, rate limit: %d",
		*addr,
		*reportInterval,
		*pollInterval,
		*rateLimit,
	)

	ctx, cancel := context.WithCancel(context.Background())
	defer cancel()

	stop := make(chan os.Signal, 1)
	signal.Notify(stop, syscall.SIGINT, syscall.SIGTERM)

	go func() {
		<-stop
		log.Println("Agent: shutting down gracefully...")
		cancel()
	}()

	a.Run(ctx)
}
