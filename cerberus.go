package cerberus

import (
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"os/exec"
	"path/filepath"
	"strings"

	"github.com/go-sharp/windows/pkg/ps"
	"golang.org/x/sys/windows/registry"
	"golang.org/x/sys/windows/svc"
	"golang.org/x/sys/windows/svc/debug"
	"golang.org/x/sys/windows/svc/eventlog"
	"golang.org/x/sys/windows/svc/mgr"
)

// DebugLogger logs all debug information, per default
// ioutil.Discard is used and no output is generated.
var DebugLogger = log.New(ioutil.Discard, "Cerberus: ", 0)

// Logger is the default logger for cerberus.
// Per default os.Stdout is used.
var Logger = log.New(os.Stdout, "Cerberus: ", 0)

// InstallService installs a windows service with the given configuration.
func InstallService(config SvcConfig) error {
	DebugLogger.Println("Open connection to service control manager...")
	manager, err := mgr.Connect()
	if err != nil {
		return newError(ErrInstallService, "failed to connect to service control manager: %v", err)
	}
	defer manager.Disconnect()

	// Ensure all required properties are set and valid.
	if err := checkAndConfigureCfg(&config); err != nil {
		return err
	}

	Logger.Printf("Installing service %v...\n", config.Name)

	DebugLogger.Printf("Creating service %v...\n", config.Name)
	cerberusPath, _ := filepath.Abs(os.Args[0]) // Consideration: pass it as argument could be a better solution
	s, err := manager.CreateService(config.Name, cerberusPath, mgr.Config{DisplayName: config.DisplayName, Description: config.Desc}, "run", config.Name)
	if err != nil {
		return newErrorW(ErrInstallService, "failed to create service", err)
	}
	defer s.Close()

	DebugLogger.Printf("Creating eventlog %v...\n", config.Name)
	if err := eventlog.InstallAsEventCreate(config.Name, eventlog.Error|eventlog.Info|eventlog.Warning); err != nil {
		s.Delete()
		return newErrorW(ErrInstallService, "failed to create eventlog %v", err, config.Name)
	}

	DebugLogger.Println("Creating configuration file...")
	if err := SaveServiceCfg(config); err != nil {
		s.Delete()
		eventlog.Remove(config.Name)
		return newErrorW(ErrInstallService, "failed to write config file", err)
	}

	Logger.Printf("Successfully installed service %v...\n", config.Name)
	return nil
}

// RemoveService removes the service with the given name.
func RemoveService(name string) error {
	DebugLogger.Println("Open connection to service control manager...")
	manager, err := mgr.Connect()
	if err != nil {
		return newErrorW(ErrRemoveService, "failed to connect to service control manager", err)
	}
	defer manager.Disconnect()

	DebugLogger.Println("Loading configuration...")
	config, err := LoadServiceCfg(name)
	if err != nil {
		return err
	}

	DebugLogger.Printf("Open service %v...\n", config.Name)
	s, err := manager.OpenService(config.Name)
	if err != nil {
		return newErrorW(ErrRemoveService, "failed to open service", err)
	}
	defer s.Close()

	Logger.Printf("Removing service %v...\n", config.Name)
	DebugLogger.Printf("Mark service %v for deletion...", config.Name)
	if err := s.Delete(); err != nil {
		return newErrorW(ErrRemoveService, "failed to remove service %v", err, config.Name)
	}

	DebugLogger.Printf("Removing eventlog %v...\n", config.Name)
	if err := eventlog.Remove(config.Name); err != nil {
		Logger.Printf("failed to remove eventlog, you might to try to remove it manually: %v\n", err)
	}

	if err := RemoveServiceCfg(config.Name); err != nil {
		Logger.Printf("Failed to remove configuration, you might try to remove it manually: %v\n", err)
	}

	Logger.Printf("Successfully removed service %v...\n", config.Name)
	return nil
}

// RunService runs the service with the given name.
func RunService(name string) error {
	isIntSess, err := svc.IsAnInteractiveSession()
	if err != nil {
		return newErrorW(ErrGeneric, "failed to determine if session is interactive", err)
	}

	DebugLogger.Println("Loading service configuration...")
	svcCfg, err := LoadServiceCfg(name)
	if err != nil {
		return err
	}

	run := svc.Run
	cerb := cerberusSvc{cfg: *svcCfg}
	if isIntSess {
		cerb.log = debug.New(svcCfg.Name)
		run = debug.Run
	} else {
		cerb.log, err = eventlog.Open(svcCfg.Name)
		if err != nil {
			return newErrorW(ErrRunService, "failed to open serivce eventlog", err)
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

func checkAndConfigureCfg(cfg *SvcConfig) error {
	DebugLogger.Println("Creating absolute path for ExePath...")
	var err error
	cfg.ExePath, err = filepath.Abs(cfg.ExePath)
	if err != nil {
		return newErrorW(ErrInstallService, "failed to get absolute path", err)
	}

	if cfg.Name == "" {
		DebugLogger.Println("Creating a service name...")
		cfg.Name = filepath.Base(cfg.ExePath)
		if idx := strings.LastIndex(cfg.Name, "."); idx == 0 {
			return newError(ErrInstallService, "invalid service name %v", cfg.Name)
		} else if idx > 0 {
			cfg.Name = cfg.Name[:idx]
		}
	}

	DebugLogger.Println("Loading configuration...")
	if _, err := LoadServiceCfg(cfg.Name); err == nil {
		return newError(ErrInstallService, " already a service (%v) installed, try to remove it first", cfg.Name)
	}

	if len(cfg.Args) > 0 {
		DebugLogger.Println("Removing leading/trailing quotes from arguments...")
		for j := range cfg.Args {
			n := len(cfg.Args[j]) - 1
			if cfg.Args[j][0] == '\'' && cfg.Args[j][n] == '\'' {
				cfg.Args[j] = cfg.Args[j][1:n]
			}
		}
	}

	if cfg.DisplayName == "" {
		DebugLogger.Println("Creating a display name..")
		cfg.DisplayName = cfg.Name
	}

	if cfg.WorkDir == "" {
		DebugLogger.Println("Setting working directory..")
		cfg.WorkDir = filepath.Dir(cfg.ExePath)
	}

	return nil
}

// SvcConfig is the data required run the executable as a service.
type SvcConfig struct {
	Name        string
	Desc        string
	DisplayName string
	ExePath     string
	WorkDir     string
	Args        []string
	Env         []string
}

type cerberusSvc struct {
	log debug.Log
	cfg SvcConfig
}

// Execute will be called when the service is started.
func (c *cerberusSvc) Execute(args []string, r <-chan svc.ChangeRequest, changes chan<- svc.Status) (svcSpecificEC bool, exitCode uint32) {
	changes <- svc.Status{State: svc.StartPending}
	cmd := exec.Cmd{Path: c.cfg.ExePath, Dir: c.cfg.WorkDir, Args: append([]string{c.cfg.ExePath}, c.cfg.Args...), Env: append(os.Environ(), c.cfg.Env...)}
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
				c.log.Error(3, fmt.Sprintf("Executable '%v' exited with error: %v", c.cfg.ExePath, err))
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

const swRegBaseKey = "SOFTWARE\\go-sharp\\cerberus"

// RemoveServiceCfg removes the service configuration form the cerberus service db.
// It returns a generic error if call fails.
func RemoveServiceCfg(name string) error {
	if name == "" {
		return newError(ErrGeneric, "empty service name is not allowed")
	}

	if err := registry.DeleteKey(registry.LOCAL_MACHINE, swRegBaseKey+"\\"+name); err != nil {
		return newErrorW(ErrGeneric, "failed to remove service entry for service '%v'", err, name)
	}

	return nil
}

// LoadServicesCfg loads all configured services.
func LoadServicesCfg() (svcs []*SvcConfig, err error) {
	key, err := registry.OpenKey(registry.LOCAL_MACHINE, swRegBaseKey, registry.QUERY_VALUE|registry.ENUMERATE_SUB_KEYS)
	if err != nil {
		return nil, newError(ErrLoadServiceCfg, "couldn't find any services")
	}

	services, err := key.ReadSubKeyNames(-1)
	if err != nil {
		return nil, newErrorW(ErrLoadServiceCfg, "failed to read services", err)
	}

	for i := range services {
		if c, err := LoadServiceCfg(services[i]); err == nil {
			svcs = append(svcs, c)
		} else {
			DebugLogger.Println("skipping item", services[i], ":", err)
		}
	}

	return svcs, nil
}

// LoadServiceCfg loads a service configuration for a given service
// from the cerberus service db.
func LoadServiceCfg(name string) (cfg *SvcConfig, err error) {
	if name == "" {
		return nil, newError(ErrLoadServiceCfg, "empty service name is not allowed")
	}

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, swRegBaseKey+"\\"+name, registry.QUERY_VALUE)
	if err != nil {
		return nil, newError(ErrLoadServiceCfg, "couldn't find service '%v'", name)
	}

	cfg = &SvcConfig{}

	if cfg.Name, _, err = key.GetStringValue("Name"); err != nil {
		return nil, newErrorW(ErrLoadServiceCfg, "failed to read name", err)
	}

	if cfg.Desc, _, err = key.GetStringValue("Desc"); err != nil {
		return nil, newErrorW(ErrLoadServiceCfg, "failed to read description", err)
	}

	if cfg.DisplayName, _, err = key.GetStringValue("DisplayName"); err != nil {
		return nil, newErrorW(ErrLoadServiceCfg, "failed to read display name", err)
	}

	if cfg.ExePath, _, err = key.GetStringValue("ExePath"); err != nil {
		return nil, newErrorW(ErrLoadServiceCfg, "failed to read exectuable path", err)
	}

	if cfg.WorkDir, _, err = key.GetStringValue("WorkDir"); err != nil {
		return nil, newErrorW(ErrLoadServiceCfg, "failed to read workdir", err)
	}

	if cfg.Args, _, err = key.GetStringsValue("Args"); err != nil {
		return nil, newErrorW(ErrLoadServiceCfg, "failed to read arguments", err)
	}

	if cfg.Env, _, err = key.GetStringsValue("Env"); err != nil {
		return nil, newErrorW(ErrLoadServiceCfg, "failed to read environment vars", err)
	}

	return cfg, nil
}

// SaveServiceCfg saves a given configuration in the cerberus service db.
func SaveServiceCfg(config SvcConfig) error {
	if config.Name == "" {
		return newError(ErrSaveServiceCfg, "empty service name is not allowed")
	}

	key, _, err := registry.CreateKey(registry.LOCAL_MACHINE, swRegBaseKey+"\\"+config.Name, registry.CREATE_SUB_KEY|registry.WRITE)
	if err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to create registry entry", err)
	}

	if err := key.SetStringValue("Name", config.Name); err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to set name", err)
	}

	if err := key.SetStringValue("Desc", config.Desc); err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to set description", err)
	}

	if err := key.SetStringValue("DisplayName", config.DisplayName); err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to set display name", err)
	}

	if err := key.SetStringValue("ExePath", config.ExePath); err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to set exectuable path", err)
	}

	if err := key.SetStringValue("WorkDir", config.WorkDir); err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to set workdir", err)
	}

	if err := key.SetStringsValue("Args", config.Args); err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to set arguments", err)
	}

	if err := key.SetStringsValue("Env", config.Env); err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to set environment vars", err)
	}

	return nil
}
