package cerberus

import (
	"encoding/json"
	"fmt"
	"io/ioutil"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/jessevdk/go-flags"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"

	"github.com/go-sharp/windows/pkg/ps"
)

const (
	configFilePath  = "cerberus.svc"
	genericCredName = "cerberus-svc-verification-key"
)

// RootCommand used for all subcommands
type RootCommand struct {
	Verbose bool `long:"verbose" short:"v" description:"Verbose output"`
}

func (r RootCommand) logDebug(format string, a ...interface{}) {
	if r.Verbose {
		r.log(format, a...)
	}
}

func (r RootCommand) log(format string, a ...interface{}) {
	fmt.Printf("Cerberus: "+format+"\n", a...)
}

// RunCommand runs the configured service directly.
type RunCommand struct {
	RootCommand
}

// Execute will run the service handler.
func (r *RunCommand) Execute(args []string) (err error) {
	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		return fmt.Errorf("Cerberus: failed to determine if session is interactive: %v", err)
	}

	r.logDebug("Loading service configuration...")
	svcCfg, err := loadServiceConfig()
	if err != nil {
		return fmt.Errorf("Cerberus: failed to load service configuration: %v", err)
	}

	run := svc.Run
	cerb := cerberusSvc{cfg: svcCfg}
	if isIntSess {
		cerb.log = debug.New(svcCfg.Name)
		run = debug.Run
	} else {
		cerb.log, err = eventlog.Open(svcCfg.Name)
		if err != nil {
			return fmt.Errorf("Cerberus: failed to open service eventlog: %v", err)
		}
	}
	defer cerb.log.Close()

	cerb.log.Info(1, fmt.Sprintf("Starting service %v ...", svcCfg.Name))
	if err := run(svcCfg.Name, &cerb); err != nil {
		cerb.log.Error(5, fmt.Sprintf("Failed to run service: %v", err))
		return err
	}

	return nil
}

// InstallCommand used to install a binary as service.
type InstallCommand struct {
	RootCommand
	ExePath     string   `long:"executable" short:"e" description:"Full path to the executable" required:"true"`
	WorkDir     string   `long:"workdir" short:"w" description:"Working directory of the executable, if not specified the folder of the executable is used."`
	Name        string   `long:"name" short:"n" description:"Name of the service, if not specified name of the executable is used."`
	DisplayName string   `long:"display-name" short:"i" description:"Display name of the service, if not specified name of the executable is used."`
	Desc        string   `long:"desc" short:"d" description:"Description of the service"`
	Args        []string `long:"arg" short:"a" description:"Arguments to pass to the executable in the same order as specified. (ex. -a \"-la\" -a \"123\")"`
}

// Execute will install a binary as service. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (i *InstallCommand) Execute(args []string) (err error) {
	i.logDebug("Open connection to service control manager...")
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("Cerberus: failed to connect to service control manager: %v", err)
	}
	defer manager.Disconnect()

	if err := i.checkAndConfigureArgs(); err != nil {
		return err
	}

	i.log("Installing service %v...", i.Name)
	svcCfg := SvcConfig{
		ExePath: i.ExePath,
		Name:    i.Name,
		WorkDir: i.WorkDir,
		Args:    i.Args,
	}

	i.logDebug("Creating configuration file...")
	if err := saveServiceConfig(svcCfg); err != nil {
		return fmt.Errorf("Cerberus: failed to write config file: %v", err)
	}

	i.logDebug("Creating service %v...", i.Name)
	cerberusPath, _ := filepath.Abs(os.Args[0])
	s, err := manager.CreateService(i.Name, cerberusPath, mgr.Config{DisplayName: i.DisplayName, Description: i.Desc}, "run")
	if err != nil {
		removeServiceConfig()
		return fmt.Errorf("Cerberus: failed to create service: %v", err)
	}
	defer s.Close()

	i.logDebug("Creating eventlog %v...", i.Name)
	if err := eventlog.InstallAsEventCreate(i.Name, eventlog.Error|eventlog.Info|eventlog.Warning); err != nil {
		s.Delete()
		removeServiceConfig()
		return fmt.Errorf("Cerberus: failed to create eventlog %v: %v", i.Name, err)
	}

	i.log("Successfully installed service %v...", i.Name)
	return nil
}

func (i *InstallCommand) checkAndConfigureArgs() error {
	i.logDebug("Creating absolute path for ExePath...")
	var err error
	i.ExePath, err = filepath.Abs(i.ExePath)
	if err != nil {
		return fmt.Errorf("Cerberus: failed to get absolute path: %v", err)
	}

	if i.Name == "" {
		i.logDebug("Creating a service name...")
		i.Name = filepath.Base(i.ExePath)
		if idx := strings.LastIndex(i.Name, "."); idx == 0 {
			return fmt.Errorf("Cerberus: invalid service name %v", i.Name)
		} else if idx > 0 {
			i.Name = i.Name[:idx]
		}
	}

	if i.DisplayName == "" {
		i.logDebug("Creating a display name..")
		i.DisplayName = i.Name
	}

	if i.WorkDir == "" {
		i.logDebug("Setting working directory..")
		i.WorkDir = filepath.Dir(i.ExePath)
	}

	i.logDebug("Loading configuration file...")
	if cfg, err := loadServiceConfig(); err == nil {
		return fmt.Errorf("Cerberus: already a service (%v) installed, try to remove it first", cfg.Name)
	}

	return nil
}

// RemoveCommand used to remove a service.
type RemoveCommand struct {
	RootCommand
	Name string `long:"name" short:"n" description:"Try to remove service by name"`
}

// Execute will remove an installed service. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (r *RemoveCommand) Execute(args []string) error {
	r.logDebug("Open connection to service control manager...")
	manager, err := mgr.Connect()
	if err != nil {
		return fmt.Errorf("Cerberus: failed to connect to service control manager: %v", err)
	}
	defer manager.Disconnect()

	if r.Name == "" {
		r.logDebug("Loading configuration file...")
		config, err := loadServiceConfig()
		if err != nil {
			return fmt.Errorf("Cerberus: failed to load configuration file, you might try to use the name parameter: %v", err)
		}
		r.Name = config.Name
	}

	s, err := manager.OpenService(r.Name)
	if err != nil {
		return fmt.Errorf("Cerberus: failed to open service: %v", err)
	}
	defer s.Close()

	r.log("Removing service %v...", r.Name)
	r.logDebug("Mark service %v for deletion...", r.Name)
	if err := s.Delete(); err != nil {
		return fmt.Errorf("Cerberus: failed to remove service %v: %v", r.Name, err)
	}

	r.logDebug("Removing eventlog %v...", r.Name)
	if err := eventlog.Remove(r.Name); err != nil {
		return fmt.Errorf("Cerberus: failed to remove eventlog %v: %v", r.Name, err)
	}

	if err := removeServiceConfig(); err != nil {
		r.log("Failed to remove config file, you might try to remove it manually: %v", err)
	}

	r.log("Successfully removed service %v...", r.Name)
	return nil
}

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
				ps.KillChildProcesses(uint32(cmd.Process.Pid), true)
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

func loadServiceConfig() (config SvcConfig, err error) {
	base, _ := filepath.Abs(os.Args[0])
	data, err := ioutil.ReadFile(filepath.Join(filepath.Dir(base), configFilePath))
	if err != nil {
		return config, err
	}

	err = json.Unmarshal(data, &config)
	return config, err
}

func removeServiceConfig() error {
	base, _ := filepath.Abs(os.Args[0])
	if err := os.Remove(filepath.Join(filepath.Dir(base), configFilePath)); err != nil {
		return err
	}
	return nil
}

func saveServiceConfig(config SvcConfig) error {
	data, err := json.Marshal(config)
	if err != nil {
		return err
	}
	base, _ := filepath.Abs(os.Args[0])
	return ioutil.WriteFile(filepath.Join(filepath.Dir(base), configFilePath), data, 0644)
}
