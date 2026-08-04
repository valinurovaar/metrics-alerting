package main

import (
	"log"

	"metrics-alerting/internal/agent"
)

func main() {
	a := agent.New(agent.DefaultAddress)
	log.Printf("Starting metrics agent, server: %s", agent.DefaultAddress)
	a.Run()
}
