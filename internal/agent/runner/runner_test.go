package runner

import (
	"context"
	"errors"
	"fmt"
	"io"
	"os"
	"path/filepath"
	"reflect"

	"strings"
	"testing"
	"time"
)

// The test binary acts as the program under test, without a shell or an installed agent.
func TestMain(m *testing.M) {
	if os.Getenv("LEGATUS_RUNNER_HELPER") == "1" {
		os.Exit(runHelper())
	}
	os.Exit(m.Run())
}

func runHelper() int {
	stdin, err := io.ReadAll(os.Stdin)
	if err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 99
	}
	if err := os.WriteFile(filepath.Join(os.Getenv("LEGATUS_RUNNER_DIR"), "stdin"), stdin, 0o600); err != nil {
		fmt.Fprintln(os.Stderr, err)
		return 99
	}
	for _, output := range []struct {
		name string
		file *os.File
	}{{"stdout", os.Stdout}, {"stderr", os.Stderr}} {
		data, err := os.ReadFile(filepath.Join(os.Getenv("LEGATUS_RUNNER_DIR"), output.name))
		if err != nil {
			fmt.Fprintln(os.Stderr, err)
			return 99
		}
		if _, err := output.file.Write(data); err != nil {
			return 99
		}
	}
	switch os.Getenv("LEGATUS_RUNNER_MODE") {
	case "exit":
		return 7
	case "wait":
		fmt.Fprintln(os.Stdout, "ready")
		time.Sleep(time.Minute)
	}
	return 0
}

func helperSpec(t *testing.T, stdout, stderr, mode string) Spec {
	t.Helper()
	exe, err := os.Executable()
	if err != nil {
		t.Fatal(err)
	}
	dir := t.TempDir()
	for name, data := range map[string]string{"stdout": stdout, "stderr": stderr} {
		if err := os.WriteFile(filepath.Join(dir, name), []byte(data), 0o600); err != nil {
			t.Fatal(err)
		}
	}
	return Spec{Exe: exe, Dir: dir, Env: append(os.Environ(),
		"LEGATUS_RUNNER_HELPER=1", "LEGATUS_RUNNER_DIR="+dir, "LEGATUS_RUNNER_MODE="+mode)}
}

func streamForTest(t *testing.T, spec Spec, onLine func([]byte)) Result {
	t.Helper()
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	res, err := Stream(ctx, spec, onLine)
	if err != nil {
		t.Fatal(err)
	}
	return res
}

func TestStdoutLinesReachCallbackInOrder(t *testing.T) {
	want := []string{"first", "  spaces stay  ", strings.Repeat("x", 128<<10), "last without newline"}
	spec := helperSpec(t, want[0]+"\n"+want[1]+"\r\n"+want[2]+"\n"+want[3], "", "")
	var got []string
	res := streamForTest(t, spec, func(line []byte) { got = append(got, string(line)) })
	if !reflect.DeepEqual(got, want) {
		t.Fatalf("stdout lines differ: got %d lines, want %d", len(got), len(want))
	}
	if res.ExitCode != 0 {
		t.Fatalf("exit code = %d, want 0", res.ExitCode)
	}
}

func TestStderrTail(t *testing.T) {
	for _, size := range []int{0, 37, stderrKeep, stderrKeep + 1, 4*stderrKeep + 123} {
		t.Run(fmt.Sprint(size), func(t *testing.T) {
			stderr := strings.Repeat("0123456789abcdef", size/16+1)[:size]
			want := stderr
			if len(want) > stderrKeep {
				want = want[len(want)-stderrKeep:]
			}
			res := streamForTest(t, helperSpec(t, "", stderr, ""), func([]byte) {})
			if res.StderrTail != want {
				t.Fatalf("stderr tail differs: got %d bytes, want %d", len(res.StderrTail), len(want))
			}
		})
	}
}

func TestNonzeroExitIsAResult(t *testing.T) {
	res := streamForTest(t, helperSpec(t, "", "failure details\n", "exit"), func([]byte) {})
	if res.ExitCode != 7 || res.StderrTail != "failure details\n" {
		t.Fatalf("result = %+v", res)
	}
}

func TestMissingProgramReturnsErrNotFound(t *testing.T) {
	const exe = "legatus-runner-test-program-that-does-not-exist"
	_, err := Stream(context.Background(), Spec{Exe: exe}, func([]byte) {})
	var missing *ErrNotFound
	if !errors.As(err, &missing) {
		t.Fatalf("error = %v, want *ErrNotFound", err)
	}
	if missing.Exe != exe {
		t.Fatalf("missing executable = %q, want %q", missing.Exe, exe)
	}
}

func TestLongPromptArrivesIntactOnStdin(t *testing.T) {
	spec := helperSpec(t, "", "", "")
	spec.Stdin = strings.Repeat("a long diff with Unicode: 世界\r\n", 20_000)
	streamForTest(t, spec, func([]byte) {})
	got, err := os.ReadFile(filepath.Join(spec.Dir, "stdin"))
	if err != nil {
		t.Fatal(err)
	}
	if string(got) != spec.Stdin {
		t.Fatalf("stdin differs: got %d bytes, want %d", len(got), len(spec.Stdin))
	}
}

func TestCancelStopsProcessPromptly(t *testing.T) {
	spec := helperSpec(t, "", "", "wait")
	ctx, cancel := context.WithTimeout(context.Background(), 20*time.Second)
	defer cancel()
	var cancelledAt time.Time
	_, err := Stream(ctx, spec, func(line []byte) {
		if string(line) == "ready" {
			cancelledAt = time.Now()
			cancel()
		}
	})
	if cancelledAt.IsZero() {
		t.Fatal("process did not signal that it was ready")
	}
	if !errors.Is(err, context.Canceled) {
		t.Fatalf("error = %v, want context.Canceled", err)
	}
	if elapsed := time.Since(cancelledAt); elapsed > 5*time.Second {
		t.Fatalf("process took %s to stop after cancellation", elapsed)
	}
}
