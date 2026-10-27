package cmd

import (
	"encoding/json"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"strconv"
	"strings"
	"text/tabwriter"
	"time"

	"github.com/spf13/cobra"

	"wrkmn/internal/control"
)

var (
	ctlJSON    bool
	ctlTail    int
	ctlTimeout time.Duration
	ctlDir     string
)

// ctlCmd drives a running wrkmn over its control socket.
var ctlCmd = &cobra.Command{
	Use:   "ctl <status|start NAME|stop NAME|restart NAME|start-all|stop-all|logs NAME>",
	Short: "Remote-control a running wrkmn (for scripts and agents)",
	Long: `Send one command to the wrkmn TUI serving this project and print its reply.

The target instance is found by project directory: the directory holding the
config (--config), --dir, or the current directory — resolved exactly as the
TUI resolves it, so run ctl from the same project (or point --config/--dir at
it). WRKMN_CONTROL_SOCKET overrides the socket path on both sides.

Commands travel the same handlers as the keybindings:
  status            every row of the process table (JSON: processes[])
  start NAME        like pressing d on NAME (dev mode + selected mode apply)
  stop NAME         like pressing x on NAME
  restart NAME      x then d
  start-all         like pressing a (dependency order, health gating); returns
                    when orchestration finishes
  stop-all          like pressing z
  logs NAME         last --tail captured log lines (default 200)

Any y/N confirm the key would raise is answered "yes". Exit status is 1 when
the reply is ok:false or no wrkmn is reachable.`,
	Args: cobra.RangeArgs(1, 2),
	RunE: runCtl,
	// Failures here are runtime conditions (no server, ok:false), not usage
	// mistakes; Execute prints the error once.
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	ctlCmd.Flags().BoolVar(&ctlJSON, "json", false, "print the raw JSON response")
	ctlCmd.Flags().IntVar(&ctlTail, "tail", control.DefaultTail, "number of log lines for `logs`")
	ctlCmd.Flags().DurationVar(&ctlTimeout, "timeout", 10*time.Minute, "how long to wait for the TUI to answer")
	ctlCmd.Flags().StringVar(&ctlDir, "dir", "", "project directory (default: --config's directory, else cwd)")
	rootCmd.AddCommand(ctlCmd)
}

// ctlBaseDir mirrors runApp's base-directory choice: --config's directory
// wins, then --dir (ctl's stand-in for the TUI's positional [directory]),
// then cwd.
func ctlBaseDir() string {
	if cfgFile != "" {
		return filepath.Dir(cfgFile)
	}
	if ctlDir != "" {
		return ctlDir
	}
	return "."
}

func buildCtlRequest(args []string) (control.Request, error) {
	req := control.Request{Cmd: args[0]}
	switch req.Cmd {
	case control.CmdStart, control.CmdStop, control.CmdRestart, control.CmdLogs:
		if len(args) != 2 {
			return req, fmt.Errorf("%s needs exactly one NAME", req.Cmd)
		}
		req.Name = args[1]
	case control.CmdStatus, control.CmdStartAll, control.CmdStopAll:
		if len(args) != 1 {
			return req, fmt.Errorf("%s takes no NAME", req.Cmd)
		}
	default:
		return req, fmt.Errorf("unknown command %q (want status, start, stop, restart, start-all, stop-all, logs)", req.Cmd)
	}
	if req.Cmd == control.CmdLogs {
		if ctlTail < 0 {
			return req, errors.New("--tail must be >= 0")
		}
		req.Tail = ctlTail
	}
	return req, req.Validate()
}

func runCtl(cmd *cobra.Command, args []string) error {
	req, err := buildCtlRequest(args)
	if err != nil {
		return err
	}
	baseDir := ctlBaseDir()
	sockPath, err := control.DefaultSocketPath(baseDir)
	if err != nil {
		return err
	}
	resp, err := control.Call(sockPath, req, ctlTimeout)
	if err != nil {
		if errors.Is(err, control.ErrNoServer) {
			absDir, _ := control.NormalizeBaseDir(baseDir)
			return fmt.Errorf("no running wrkmn for %s; socket %s", absDir, sockPath)
		}
		return err
	}
	if ctlJSON {
		raw, _ := json.Marshal(resp)
		fmt.Println(string(raw))
	} else if err := printCtlHuman(req, resp); err != nil {
		return err
	}
	if !resp.OK {
		if ctlJSON {
			// The JSON on stdout already carries the error.
			os.Exit(1)
		}
		return errors.New(resp.Error)
	}
	return nil
}

func printCtlHuman(req control.Request, resp control.Response) error {
	if !resp.OK {
		return nil // the error is returned (and printed) by runCtl
	}
	switch req.Cmd {
	case control.CmdStatus, control.CmdStartAll, control.CmdStopAll:
		var status control.StatusResult
		if err := json.Unmarshal(resp.Result, &status); err != nil {
			return fmt.Errorf("decoding status: %w", err)
		}
		printStatusTable(status)
	case control.CmdLogs:
		var logs control.LogsResult
		if err := json.Unmarshal(resp.Result, &logs); err != nil {
			return fmt.Errorf("decoding logs: %w", err)
		}
		for _, line := range logs.Lines {
			fmt.Println(line)
		}
	default:
		var action control.ActionResult
		if err := json.Unmarshal(resp.Result, &action); err != nil {
			return fmt.Errorf("decoding result: %w", err)
		}
		fmt.Printf("%s %s: %s\n", req.Cmd, action.Name, action.Status)
	}
	return nil
}

func printStatusTable(status control.StatusResult) {
	tw := tabwriter.NewWriter(os.Stdout, 2, 4, 2, ' ', 0)
	fmt.Fprintln(tw, "NAME\tSTATUS\tPORT\tUPTIME\tTYPE\tDEV MODE\tMODE\tPATH")
	for _, r := range status.Processes {
		port := "-"
		if r.Port > 0 {
			port = strconv.Itoa(r.Port)
		}
		uptime := "-"
		if r.UptimeSeconds > 0 {
			uptime = (time.Duration(r.UptimeSeconds) * time.Second).String()
		}
		devMode, mode := "-", "-"
		if r.Type == "worker" {
			devMode = r.DevMode
			if r.SelectedMode != "" {
				mode = r.SelectedMode
			} else {
				mode = "(cf default)"
			}
		}
		fmt.Fprintf(tw, "%s\t%s\t%s\t%s\t%s\t%s\t%s\t%s\n",
			r.Name, r.Status, port, uptime, r.Type, devMode, mode, strings.TrimSpace(r.Path))
	}
	tw.Flush()
}
