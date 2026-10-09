package cli

import (
	"bytes"
	"context"
	"fmt"
	"io"
	"net/http"
	"time"

	"github.com/Mvnshi/legatus/internal/model"
	"github.com/Mvnshi/legatus/internal/server"
)

// cmdCancel stops a run that has not finished. With the daemon running the daemon does it (it may be in
// the middle of the run); without one the run's record is simply marked cancelled.
func cmdCancel(args []string, stdout, stderr io.Writer) int {
	fs := newFlags("cancel", stderr)
	root := fs.String("root", "", "where Legatus keeps its files")
	if code, ok := parse(fs, args); !ok {
		return code
	}
	if fs.NArg() != 1 {
		fmt.Fprintln(stderr, "Usage: legatus cancel <run id>")
		return 64
	}
	id := fs.Arg(0)
	a, ok := openApp(*root, stderr)
	if !ok {
		return 1
	}
	if token, err := server.LoadOrCreateToken(a.Root); err == nil {
		if d, err := server.ReadDaemon(a.Root); err == nil && server.Ping(d.Addr, token) {
			req, _ := http.NewRequest("POST", "http://"+d.Addr+"/api/runs/"+id+"/cancel", bytes.NewReader([]byte("{}")))
			req.Header.Set("Content-Type", "application/json")
			req.Header.Set("X-Legatus-Token", token)
			resp, err := (&http.Client{Timeout: 30 * time.Second}).Do(req)
			if err != nil {
				return fail(stderr, err)
			}
			defer resp.Body.Close()
			if resp.StatusCode != http.StatusOK {
				body, _ := io.ReadAll(io.LimitReader(resp.Body, 4096))
				return fail(stderr, fmt.Errorf("the daemon refused: %s", bytes.TrimSpace(body)))
			}
			fmt.Fprintf(stdout, "Canceled %s.\n", id)
			return 0
		}
	}
	run, err := a.Store.Load(id)
	if err != nil {
		return fail(stderr, err)
	}
	if run.Status.Terminal() {
		return fail(stderr, fmt.Errorf("run %s is already %s", id, run.Status))
	}
	if run.Status == model.NeedsHuman {
		return fail(stderr, fmt.Errorf("run %s is waiting for a person; open it in the cockpit to retry or leave it", id))
	}
	if err := a.Engine.MarkCanceled(context.Background(), id); err != nil {
		return fail(stderr, err)
	}
	fmt.Fprintf(stdout, "Canceled %s. Its branch and worktree are kept; legatus clean removes finished worktrees.\n", id)
	return 0
}
