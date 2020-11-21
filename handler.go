package cerberus

import (
	"fmt"
	"os"
	"os/exec"
	"time"

	"github.com/go-sharp/windows/pkg/signal"

	"github.com/go-sharp/windows/pkg/ps"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
)

type cerberusSvc struct {
	log  debug.Log
	cfg  SvcConfig
	cmd  *exec.Cmd
	done chan error
	// Restart Counter
	restarts    int
	lastRestart time.Time
}

type recoveryHandlerStatus int

const (
	rerunServiceStatus recoveryHandlerStatus = iota
	shutdownGracefullyStatus
	errorStatus
)

// Execute will be called when the service is started.
func (c *cerberusSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	changes <- svc.Status{State: svc.StartPending}

	// Setup signaling for the process and run it
	c.done = make(chan error)
	err := c.runSvc()
	if err != nil {
		c.log.Error(2, err.Error())
		return false, 2
	}

	changes <- svc.Status{State: svc.Running, Accepts: svc.AcceptStop | svc.AcceptShutdown}
	c.log.Info(1, fmt.Sprintf("Service %v is running...", c.cfg.Name))

loop:
	for {
		select {
		case err := <-c.done:
			if err != nil {
				c.log.Error(3, fmt.Sprintf("Executable '%v' exited with error: %v", c.cfg.ExePath, err))
				// Check if we have a proper exit error and act according configuration
				if e, ok := err.(*exec.ExitError); ok {
					ec := e.ExitCode()
					// If we get -1 process was stopped by a signal, so we stopping gracefully.
					if ec < 0 {
						break loop
					}
					// Check if any recovery action is defined an handle it accordingly.
					if action, ok := c.cfg.RecoveryActions[ec]; ok {
						switch c.handleRecovery(action) {
						case rerunServiceStatus:
							continue
						case shutdownGracefullyStatus:
							break loop
						default:
							// If we get here we stop the service and log an error.
						}
					}
				}
				c.log.Error(3, fmt.Sprintf("Service %v unexpectedly stopped...", c.cfg.Name))
				// We return here so the SCM knows that an error occured
				return false, 3
			}
			break loop

		case cr := <-r:
			switch cr.Cmd {
			case svc.Interrogate:
				changes <- cr.CurrentStatus
			case svc.Shutdown, svc.Stop:
				changes <- svc.Status{State: svc.StopPending}
				c.log.Info(1, "Received shutdown command, shutting down...")
				c.shutdown(changes)
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

func (c *cerberusSvc) shutdown(ch chan<- svc.Status) {
	sig := c.cfg.StopSignal
	if sig > NoSignal {
		// Sending WM_QUIT if configured
		if sig&WmQuitSignal == WmQuitSignal {
			if err := signal.SendSignal(uint32(c.cmd.Process.Pid), signal.WmQuit); err != nil {
				c.log.Warning(1, fmt.Sprintf("Failed to send WM_QUIT signal: %v", err))
			}
		}
		// Sending WM_CLOSE if configured
		if sig&WmCloseSignal == WmCloseSignal {
			if err := signal.SendSignal(uint32(c.cmd.Process.Pid), signal.WmClose); err != nil {
				c.log.Warning(1, fmt.Sprintf("Failed to send WM_QUIT signal: %v", err))
			}
		}

		// Sending Ctrl-C if configured
		if sig&CtrlCSignal == CtrlCSignal {
			if err := signal.SendCtrlEvent(uint32(c.cmd.Process.Pid), signal.CtrlCEvent); err != nil {
				c.log.Warning(1, fmt.Sprintf("Failed to send Ctrl-C signal: %v", err))
			}
		}

		// If the process doesn't stop within 30 seconds we will kill the process.
		select {
		case <-time.After(time.Second * 30):
		case <-c.done:
			return
		}
	}

	ps.KillChildProcesses(uint32(c.cmd.Process.Pid), true)
	<-c.done
}

func (c *cerberusSvc) handleRecovery(action SvcRecoveryAction) recoveryHandlerStatus {
	c.log.Info(3, "Applying defined recovery action...")
	// We stop the service if no action is defined
	if action.Action == NoAction {
		c.log.Info(3, "Shutdown service gracefully ...")
		return shutdownGracefullyStatus
	}
	// Check if we have to run a external program
	if action.Action&RunProgramAction == RunProgramAction {
		c.log.Info(3, fmt.Sprintf("Executing defined program '%v'...", action.Program))
		if err := exec.Command(action.Program, action.Arguments...).Start(); err != nil {
			c.log.Error(3, fmt.Sprintf("Failed to start external program '%v': %v", action.Program, err))
			return errorStatus
		}
	}

	// Check if we should restart the program
	if action.Action&RestartAction == RestartAction {
		// We reset the counter if the specified period has elapsed.
		if !c.lastRestart.IsZero() && time.Now().Sub(c.lastRestart) > action.ResetAfter {
			c.log.Info(3, "Resetting restart counter...")
			c.restarts = 0
		}

		// If we get here we should restart the service as long as max restarts not exceeds the limit.
		if action.MaxRestarts > 0 && c.restarts >= action.MaxRestarts {
			c.log.Error(3, fmt.Sprintf("Executable '%v' reached specified restart limits: %v", c.cfg.ExePath, action.MaxRestarts))
			return errorStatus
		}

		c.restarts++
		c.lastRestart = time.Now()
		// Waiting for the restart
		if action.Delay > 0 {
			time.Sleep(time.Duration(action.Delay) * time.Second)
		}

		c.log.Info(3, fmt.Sprintf("Restarting service %v", c.cfg.Name))
		if err := c.runSvc(); err != nil {
			c.log.Error(3, err.Error())
			return errorStatus
		}

		// We continue the loop
		return rerunServiceStatus
	}
	return errorStatus
}

func (c *cerberusSvc) runSvc() error {
	c.cmd = &exec.Cmd{Path: c.cfg.ExePath, Dir: c.cfg.WorkDir, Args: append([]string{c.cfg.ExePath}, c.cfg.Args...), Env: append(os.Environ(), c.cfg.Env...)}
	if err := c.cmd.Start(); err != nil {
		return fmt.Errorf("Failed to start service: %v", err)
	}

	go func() {
		c.done <- c.cmd.Wait()
	}()

	return nil
}
