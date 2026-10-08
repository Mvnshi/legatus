package engine

import (
	"context"
	"errors"
	"fmt"
	"os"
	"path/filepath"
	"regexp"
	"strings"

	"github.com/Mvnshi/legatus/internal/github"
	"github.com/Mvnshi/legatus/internal/model"
)

var (
	shaLike    = regexp.MustCompile(`^[0-9a-f]{7,40}$`)
	issueSrc   = regexp.MustCompile(`^github:([A-Za-z0-9][A-Za-z0-9-]*/[A-Za-z0-9._-]+)#(\d+)$`)
	branchName = regexp.MustCompile(`^[A-Za-z0-9._/-]+$`)
)

// baseBranch is the branch a pull request should target, or "" to use the repository's default. A task that
// started from HEAD or a commit has no branch to name.
func baseBranch(base string) string {
	b := strings.TrimSpace(base)
	if b == "" || b == "HEAD" || shaLike.MatchString(b) || !branchName.MatchString(b) {
		return ""
	}
	return b
}

// OpenPullRequest pushes the run's branch and opens a pull request whose description is the run's report.
// It is only for runs that finished their work (a run that needs a person gets a draft). Calling it again for
// a run that already has one returns that one.
func (e *Engine) OpenPullRequest(ctx context.Context, id string, draft bool) (string, error) {
	run, err := e.Store.Load(id)
	if err != nil {
		return "", err
	}
	if run.PRURL != "" {
		return run.PRURL, nil
	}
	switch run.Status {
	case model.Succeeded:
	case model.NeedsHuman:
		draft = true // a person has not agreed with this work yet
	default:
		return "", fmt.Errorf("run %s is %s; a pull request can be opened once it has succeeded", id, run.Status)
	}
	dir, err := e.Store.RunDir(id)
	if err != nil {
		return "", err
	}
	report, err := os.ReadFile(filepath.Join(dir, "evidence.md"))
	if err != nil {
		return "", errors.New("this run has no report to use as the pull request description")
	}
	body := string(report)
	if m := issueSrc.FindStringSubmatch(run.Task.Source); m != nil && run.Status == model.Succeeded {
		body = "Closes " + m[1] + "#" + m[2] + "\n\n" + body
	}
	var gh PROpener = e.GitHub
	if gh == nil {
		gh = &github.Client{}
	}
	url, err := gh.OpenPR(ctx, github.PROptions{
		Dir: run.Worktree, Branch: run.Branch, Base: baseBranch(run.Task.Base),
		Title: run.Task.Title, Body: body, Draft: draft,
	})
	if err != nil {
		e.emit(id, "", "pr.failed", map[string]any{"error": clip(err.Error(), 600)})
		return "", err
	}
	// Reload: the run may have been saved by someone else while the push was under way.
	if cur, lerr := e.Store.Load(id); lerr == nil {
		run = cur
	}
	run.PRURL = url
	if err := e.save(run); err != nil {
		return url, err
	}
	e.emit(id, "", "pr.opened", map[string]any{"url": url, "draft": draft})
	return url, nil
}
