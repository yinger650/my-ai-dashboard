// Package wrap runs a local command and registers it with board-client.
package wrap

import (
	"fmt"
	"io"
	"os"
	"os/exec"
	"strings"
	"time"

	"agentboard/internal/client/config"
	"agentboard/internal/client/control"
	"agentboard/internal/shared"
)

// Options for wrapping a process.
type Options struct {
	ConfigPath string
	Sock       string
	Summary    string
	TTL        time.Duration
	LogPath    string
	Stdout     io.Writer
	Stderr     io.Writer
	Stdin      io.Reader
}

// ParseArgs splits wrap flags from the command after `--`.
func ParseArgs(args []string) (Options, []string, error) {
	var opt Options
	i := 0
	for i < len(args) {
		a := args[i]
		if a == "--" {
			return opt, args[i+1:], nil
		}
		switch a {
		case "--summary":
			if i+1 >= len(args) {
				return opt, nil, fmt.Errorf("--summary requires a value")
			}
			i++
			opt.Summary = args[i]
		case "--ttl":
			if i+1 >= len(args) {
				return opt, nil, fmt.Errorf("--ttl requires a value")
			}
			i++
			d, err := time.ParseDuration(args[i])
			if err != nil {
				return opt, nil, fmt.Errorf("--ttl: %w", err)
			}
			opt.TTL = d
		case "--log":
			if i+1 >= len(args) {
				return opt, nil, fmt.Errorf("--log requires a path")
			}
			i++
			opt.LogPath = args[i]
		case "--config", "-c":
			if i+1 >= len(args) {
				return opt, nil, fmt.Errorf("--config requires a path")
			}
			i++
			opt.ConfigPath = args[i]
		case "-h", "--help":
			return opt, nil, fmt.Errorf("usage: board-client wrap --summary TEXT [--ttl 6h] [--log PATH] [--config client.yaml] -- CMD...")
		default:
			return opt, nil, fmt.Errorf("unknown flag %q (put the command after --)", a)
		}
		i++
	}
	return opt, nil, fmt.Errorf("missing command; use -- CMD...")
}

// SockPath resolves the control socket from config or a default.
func SockPath(cfgPath, override string) string {
	if override != "" {
		return override
	}
	if cfgPath == "" {
		cfgPath = os.Getenv("BOARD_CLIENT_CONFIG")
	}
	if cfgPath == "" {
		cfgPath = "/etc/agentboard/client.yaml"
	}
	c, err := config.Read(cfgPath)
	if err != nil {
		return "/var/lib/agentboard-client/control.sock"
	}
	return c.ControlSockPath()
}

// Run execs argv, registers the pid, and returns the process exit code.
func Run(opt Options, argv []string) (int, error) {
	if len(argv) == 0 {
		return 2, fmt.Errorf("empty command")
	}
	if opt.Stdout == nil {
		opt.Stdout = os.Stdout
	}
	if opt.Stderr == nil {
		opt.Stderr = os.Stderr
	}
	if opt.Stdin == nil {
		opt.Stdin = os.Stdin
	}
	if strings.TrimSpace(opt.Summary) == "" {
		opt.Summary = strings.Join(argv, " ")
	}
	runKey := shared.NewID()
	sock := SockPath(opt.ConfigPath, opt.Sock)
	sess, err := control.Dial(sock, 2*time.Second)
	if err != nil {
		fmt.Fprintf(opt.Stderr, "board-client wrap: daemon not running (%v); running command anyway\n", err)
		sess = nil
	} else {
		defer sess.Close()
	}

	cmd := exec.Command(argv[0], argv[1:]...)
	cmd.Stdin = opt.Stdin
	cmd.Stderr = opt.Stderr
	var stdoutR *io.PipeReader
	var stdoutW *io.PipeWriter
	if opt.LogPath == "" && sess != nil {
		stdoutR, stdoutW = io.Pipe()
		cmd.Stdout = io.MultiWriter(opt.Stdout, stdoutW)
	} else {
		cmd.Stdout = opt.Stdout
	}
	if err := cmd.Start(); err != nil {
		if stdoutW != nil {
			_ = stdoutW.Close()
		}
		return 1, err
	}
	if sess != nil {
		ttl := int(opt.TTL.Seconds())
		resp, err := sess.Do(control.Request{
			Op:         "wrap_start",
			RunKey:     runKey,
			PID:        cmd.Process.Pid,
			Summary:    opt.Summary,
			TTLSeconds: ttl,
			LogPath:    opt.LogPath,
		}, 5*time.Second)
		if err != nil || !resp.OK {
			fmt.Fprintf(opt.Stderr, "board-client wrap: register failed (%v %s)\n", err, resp.Error)
		}
	}
	if stdoutR != nil {
		go copyStdout(sess, runKey, stdoutR)
	}
	waitErr := cmd.Wait()
	if stdoutW != nil {
		_ = stdoutW.Close()
	}
	code := 0
	if waitErr != nil {
		if ee, ok := waitErr.(*exec.ExitError); ok {
			code = ee.ExitCode()
		} else {
			return 1, waitErr
		}
	}
	if sess != nil {
		_, _ = sess.Do(control.Request{Op: "wrap_exit", RunKey: runKey, ExitCode: &code}, 5*time.Second)
	}
	return code, nil
}

func copyStdout(sess *control.Session, runKey string, r io.Reader) {
	buf := make([]byte, 32*1024)
	for {
		n, err := r.Read(buf)
		if n > 0 && sess != nil {
			_, _ = sess.Do(control.Request{Op: "wrap_stdout", RunKey: runKey, Chunk: string(buf[:n])}, 5*time.Second)
		}
		if err != nil {
			return
		}
	}
}
