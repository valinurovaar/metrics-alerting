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

	flag.Parse()

	if envAddr := os.Getenv("ADDRESS"); envAddr != "" {
		*addr = envAddr
	}

	if envReport := os.Getenv("REPORT_INTERVAL"); envReport != "" {
		if value, err := strconv.Atoi(envReport); err == nil {
			*reportInterval = value
		} else {
			log.Printf("Warning: не удалось распарсить переменную окружения REPORT_INTERVAL (%s), используется значение по умолчанию: %d", envReport, *reportInterval)
		}
	}

	if envPoll := os.Getenv("POLL_INTERVAL"); envPoll != "" {
		if value, err := strconv.Atoi(envPoll); err == nil {
			*pollInterval = value
		} else {
			log.Printf("Warning: не удалось распарсить переменную окружения POLL_INTERVAL (%s), используется значение по умолчанию: %d", envPoll, *pollInterval)
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