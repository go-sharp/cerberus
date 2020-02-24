package cerberus

import (
	"fmt"
	"os/exec"

	"github.com/jessevdk/go-flags"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

// CommandFunc takes a function and wraps into a type which implements the commander interface.
func CommandFunc(f func(args []string) error) flags.Commander {
	return &funcCommand{fn: f}
}

type funcCommand struct {
	fn func(args []string) error
}

func (c funcCommand) Execute(args []string) error {
	return c.fn(args)
}

// SvcConfig is the data required run the executable as a service.
type SvcConfig struct {
	Name        string   `json:"name,omitempty"`
	ExePath     string   `json:"exe_path,omitempty"`
	WorkDir     string   `json:"work_dir,omitempty"`
	Args        []string `json:"args,omitempty"`
	WaitTimeout int      `json:"wait_timeout,omitempty"`
}

type cerberusSvc struct {
	log debug.Log
	cfg SvcConfig
}

// Execute will be called when the service is started.
func (c *cerberusSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	changes <- svc.Status{State: svc.StartPending}
	cmd := exec.Cmd{Path: c.cfg.ExePath, Dir: c.cfg.WorkDir, Args: c.cfg.Args}
	if err := cmd.Start(); err != nil {
		c.log.Error(2, fmt.Sprintf("Failed to start service: %v", err))
		return false, 2
	}

	// Setup signaling for the process and run it
	done := make(chan error)
	go func() {
		done <- cmd.Wait()
	}()

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	c.log.Info(1, fmt.Sprintf("Service %v is running...", c.cfg.Name))

loop:
	for {
		select {
		case err := <-done:
			if err != nil {
				c.log.Error(3, fmt.Sprintf("Executable exited with error: %v", err))
				exitCode = 3
			}
			break loop

		case cr := <-r:
			switch cr.Cmd {
			case svc.Interrogate:
				changes <- cr.CurrentStatus
			case svc.Shutdown, svc.Stop:
				changes <- svc.Status{State: svc.StopPending}
				c.log.Info(1, "Received shutdown command, shutting down...")
				cmd.Process.Kill()
				<-done
				break loop
			default:
				c.log.Warning(4, fmt.Sprintf("Unexpected control sequence received: #%d", cr))
			}
		}
	}

	changes <- svc.Status{State: svc.Stopped}
	c.log.Info(1, fmt.Sprintf("Service %v stopped...", c.cfg.Name))
	return
}
