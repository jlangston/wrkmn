package ui

import (
	"wrkmn/internal/worker"
)

// Legacy view mode constants for backwards compatibility
const (
	WorkerListView = ViewDashboard
	LogsView       = ViewLogs
)

func NewModel(workers []*worker.Worker) TableModel {
	return NewTableModel(workers)
}
