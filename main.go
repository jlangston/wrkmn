package main

import (
	"fmt"
	"log"
	"os"
	"os/signal"
	"syscall"

	"cf-worker-manager/internal/ui"
	"cf-worker-manager/internal/worker"

	tea "github.com/charmbracelet/bubbletea"
)

func main() {
	workersPath := "workers"
	if len(os.Args) > 1 {
		workersPath = os.Args[1]
	}

	discoverer := worker.NewDiscoverer()
	workers, err := discoverer.DiscoverWorkers(workersPath)
	if err != nil {
		log.Fatalf("Failed to discover workers: %v", err)
	}

	if len(workers) == 0 {
		fmt.Printf("No workers found in %s\n", workersPath)
		os.Exit(1)
	}

	m := ui.NewModel(workers)
	p := tea.NewProgram(m, tea.WithAltScreen())

	// Set up signal handling for graceful shutdown
	c := make(chan os.Signal, 1)
	signal.Notify(c, os.Interrupt, syscall.SIGTERM)

	go func() {
		<-c
		// Gracefully shutdown all processes
		m.GetManager().StopAll()
		p.Kill()
		os.Exit(0)
	}()

	if _, err := p.Run(); err != nil {
		log.Fatalf("Error running program: %v", err)
	}
}
