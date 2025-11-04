# Cloudflare Workers Manager

A TUI (Terminal User Interface) for managing your Cloudflare Workers locally, built with Go.

## Features

- 📁 **Auto-discovery**: Automatically discovers all Cloudflare workers in a directory
- 📦 **Package.json parsing**: Reads worker names and scripts from package.json files
- 🚀 **Process management**: Start/stop workers individually or all at once
- 📊 **Real-time status**: Visual status indicators for each worker
- 📄 **Live logs**: View real-time logs for running workers (like k9s)
- 🎯 **Port detection**: Automatically extracts dev ports from scripts

## Installation

```bash
go build -o cf-worker-manager
```

## Usage

Run from your workers directory:
```bash
./cf-worker-manager
```

Or specify a custom path:
```bash
./cf-worker-manager /path/to/your/workers
```

## Controls

### Worker List View
- `d` - Start worker in dev mode
- `s` - Start worker with start script  
- `x` - Stop selected worker
- `a` - Start all workers
- `z` - Stop all workers
- `l` / `Enter` - View logs for selected worker
- `r` - Refresh worker list
- `q` / `Ctrl+C` - Quit

### Logs View
- `↑/↓` - Scroll through logs
- `q` / `Esc` - Back to worker list
- `Ctrl+C` - Quit

## Status Indicators

- 🟢 Green dot - Worker is running
- ⚫ Gray dot - Worker is stopped
- 🔴 Red dot - Worker failed to start
- 🟡 Yellow dot - Worker is starting

## Project Structure

```
├── main.go                    # Application entry point
├── internal/
│   ├── worker/
│   │   ├── types.go          # Worker and PackageJSON types
│   │   └── discoverer.go     # Worker discovery logic
│   ├── process/
│   │   └── manager.go        # Process management and logging
│   └── ui/
│       ├── model.go          # Main TUI model and event handling
│       └── logs.go           # Log viewing component
└── go.mod
```

## Requirements

- Go 1.24+
- Workers with `package.json` containing `wrangler` scripts

## Supported Scripts

The manager looks for these scripts in your `package.json`:
- `dev` - Development mode (preferred)
- `start` - Start command
- `deploy` - Deploy command
- `test` - Test command
- `cf-typegen` - Type generation

Port detection works by parsing `--port` flags in dev scripts.