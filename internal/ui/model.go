package ui

import (
	"cf-worker-manager/internal/worker"
)

type ViewMode int

const (
	WorkerListView ViewMode = iota
	LogsView
)

func NewModel(workers []*worker.Worker) TableModel {
	return NewTableModel(workers)
}
