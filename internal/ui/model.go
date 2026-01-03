package ui

import (
	"wrkmn/internal/config"
	"wrkmn/internal/worker"
)

// Legacy view mode constants for backwards compatibility
const (
	WorkerListView = ViewDashboard
	LogsView       = ViewLogs
)

func NewModel(workers []*worker.Worker) TableModel {
	return NewTableModel(workers, nil)
}

func NewModelWithConfig(workers []*worker.Worker, cfg *config.Config) TableModel {
	return NewTableModel(workers, cfg)
}
