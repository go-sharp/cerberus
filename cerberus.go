package cerberus

import (
	"bytes"
	"encoding/gob"
	"fmt"
	"io/ioutil"
	"log"
	"os"
	"path/filepath"
	"strings"
	"time"

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
		return newError(ErrSCMConnect, "failed to connect to service control manager: %v", err)
	}
	defer manager.Disconnect()

	// Ensure all required properties are initalized.
	if err := initConfiguration(&config); err != nil {
		return err
	}
	// Validate all properties
	if err := validateConfiguration(manager, &config); err != nil {
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

	DebugLogger.Println("Write service configuration...")
	if err := saveServiceCfg(config); err != nil {
		s.Delete()
		eventlog.Remove(config.Name)
		return err
	}

	Logger.Printf("Successfully installed service %v...\n", config.Name)
	return nil
}

// UpdateService updates a cerberus service with the given configuration.
func UpdateService(config SvcConfig) error {
	DebugLogger.Println("Open connection to service control manager...")
	manager, err := mgr.Connect()
	if err != nil {
		return newError(ErrSCMConnect, "failed to connect to service control manager: %v", err)
	}
	defer manager.Disconnect()

	DebugLogger.Println("Loading configuration...")
	currentSvc, err := LoadServiceCfg(config.Name)
	if err != nil {
		return err
	}

	Logger.Printf("Updating service %v...\n", config.Name)
	trimArgs(config.Args)
	currentSvc.Args = config.Args
	currentSvc.Desc = config.Desc
	currentSvc.DisplayName = config.DisplayName
	currentSvc.Env = config.Env
	currentSvc.RecoveryActions = config.RecoveryActions
	currentSvc.WorkDir = config.WorkDir

	// Validate all properties
	if err := validateConfiguration(manager, &config); err != nil {
		return err
	}

	DebugLogger.Println("Write service configuration...")
	if err := saveServiceCfg(config); err != nil {
		return err
	}

	Logger.Printf("Successfully updated service %v...\n", config.Name)
	return nil
}

// RemoveService removes the service with the given name.
// Stops the service first, can return a timeout error if it can't stop the service.
func RemoveService(name string) error {
	DebugLogger.Println("Open connection to service control manager...")
	manager, err := mgr.Connect()
	if err != nil {
		return newErrorW(ErrSCMConnect, "failed to connect to service control manager", err)
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

	DebugLogger.Printf("Stopping service %v...\n", config.Name)
	s.Control(svc.Stop)
	timeout := time.Now().Add(30 * time.Second)
	state, _ := s.Query()
	for state.State != svc.Stopped {
		if time.Now().After(timeout) {
			return newError(ErrTimeout, "failed to stop service")
		}

		time.Sleep(200 * time.Millisecond)
		state, _ = s.Query()
	}

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

func validateConfiguration(m *mgr.Mgr, cfg *SvcConfig) error {
	DebugLogger.Println("Validating configuration...")
	if cfg.Name == "" {
		return newError(ErrInvalidConfiguration, "service name can't be empty")
	}

	if cfg.ExePath == "" {
		return newError(ErrInvalidConfiguration, "executable path can't be empty")
	}

	if fi, err := os.Stat(cfg.ExePath); err != nil || fi.IsDir() {
		return newErrorW(ErrInvalidConfiguration, "executable path isn't a binary file", err)
	}

	for _, action := range cfg.RecoveryActions {
		if (action.Action & RunProgramAction) == RunProgramAction {
			if action.Program == "" {
				return newError(ErrInvalidConfiguration, "recovery action program path can't be empty")
			}
			if fi, err := os.Stat(action.Program); err != nil || fi.IsDir() {
				return newErrorW(ErrInvalidConfiguration, "recovery action program path isn't a binary file", err)
			}
		}
	}

	if len(cfg.Dependencies) > 0 {
		services, err := m.ListServices()
		if err != nil {
			return newErrorW(ErrGeneric, "failed to get service list", err)
		}
		for i := range cfg.Dependencies {
			found := false
			for j := range services {
				if cfg.Dependencies[i] == services[j] {
					found = true
					break
				}
			}
			if !found {
				return newError(ErrInvalidConfiguration, "couldn't find a dependency: "+cfg.Dependencies[i])
			}
		}
	}

	return nil
}

func initConfiguration(cfg *SvcConfig) error {
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

	trimArgs(cfg.Args)

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
	// Base configuration
	Name        string
	Desc        string
	DisplayName string
	ExePath     string
	WorkDir     string
	Args        []string
	Env         []string

	// Extended Configurations
	RecoveryActions map[int]SvcRecoveryAction
	Dependencies    []string
	ServiceUser     string
	Password        *string
	StartType       StartType
}

// StartType configures the startup type.
type StartType uint32

const (
	// AutoStartType configures the service to startup automatically.
	AutoStartType StartType = 2
	// AutoDelayedStartType configures the service to startup automatically with delay.
	AutoDelayedStartType StartType = 9999
	// ManualStartType configures the service for manual startup.
	ManualStartType StartType = 3
	// DisabledStartType disables the service.
	DisabledStartType StartType = 4
)

// RecoveryAction defines what happens if a binary exits with error
type RecoveryAction int

const (
	// NoAction is the default action and will stop the service on error.
	NoAction RecoveryAction = 1 << iota
	// RestartAction restarts the serivce according the specified settings.
	RestartAction
	// RunProgramAction will run the specified program.
	RunProgramAction

	// RunAndRestartAction restarts the service and runs the specified program.
	RunAndRestartAction = RestartAction | RunProgramAction
)

// SvcRecoveryAction defines what cerberus should do if a binary returns an error.
type SvcRecoveryAction struct {
	ExitCode    int
	Action      RecoveryAction
	Delay       int
	MaxRestarts int
	ResetAfter  time.Duration
	Program     string
	Arguments   []string
}

const swRegBaseKey = "SOFTWARE\\go-sharp\\cerberus\\services"

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
	DebugLogger.Println("Loading service configuration for " + name + "...")
	if name == "" {
		return nil, newError(ErrLoadServiceCfg, "empty service name is not allowed")
	}

	key, err := registry.OpenKey(registry.LOCAL_MACHINE, swRegBaseKey+"\\"+name, registry.QUERY_VALUE)
	if err != nil {
		return nil, newError(ErrLoadServiceCfg, "couldn't find service '%v'", name)
	}

	manager, err := mgr.Connect()
	if err != nil {
		return nil, newErrorW(ErrSCMConnect, "failed to connect to service control manager", err)
	}
	defer manager.Disconnect()

	svc, err := manager.OpenService(name)
	if err != nil {
		return nil, newErrorW(ErrSaveServiceCfg, "failed to load serivce from scm", err)
	}

	scmCfg, err := svc.Config()
	if err != nil {
		return nil, newErrorW(ErrGeneric, "failed to get service configuration from scm", err)
	}

	cfg = &SvcConfig{
		ServiceUser:  scmCfg.ServiceStartName,
		Dependencies: scmCfg.Dependencies,
	}

	if scmCfg.DelayedAutoStart && StartType(scmCfg.StartType) == AutoStartType {
		cfg.StartType = AutoDelayedStartType
	} else {
		cfg.StartType = StartType(scmCfg.StartType)
	}

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

	if data, _, err := key.GetBinaryValue("RecoveryActions"); err == nil {
		dec := gob.NewDecoder(bytes.NewReader(data))
		if err := dec.Decode(&cfg.RecoveryActions); err != nil {
			return nil, newErrorW(ErrLoadServiceCfg, "failed to read recovery actions", err)
		}
	} else {
		cfg.RecoveryActions = map[int]SvcRecoveryAction{}
	}

	return cfg, nil
}

func updateSCMProperties(cfg *SvcConfig) error {
	DebugLogger.Println("Updating SCM service properties...")
	manager, err := mgr.Connect()
	if err != nil {
		return newErrorW(ErrSCMConnect, "failed to connect to service control manager", err)
	}
	defer manager.Disconnect()

	svc, err := manager.OpenService(cfg.Name)
	if err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to load serivce from scm", err)
	}

	config, err := svc.Config()
	if err != nil {
		return newErrorW(ErrGeneric, "failed to get service configuration from scm", err)
	}

	if cfg.StartType == AutoDelayedStartType {
		config.StartType = mgr.StartAutomatic
		config.DelayedAutoStart = true
	} else {
		config.StartType = uint32(cfg.StartType)
	}

	config.Dependencies = cfg.Dependencies
	if len(config.Dependencies) == 0 {
		config.Dependencies = []string{"\x00"}
	}

	if cfg.ServiceUser == "" {
		config.ServiceStartName = "LocalSystem"
	} else {
		config.ServiceStartName = cfg.ServiceUser
	}

	if cfg.Password != nil {
		config.Password = *cfg.Password
	}

	if err := svc.UpdateConfig(config); err != nil {
		return newErrorW(ErrSaveServiceCfg, "failed to update scm properties", err)
	}

	return nil
}

// saveServiceCfg saves a given configuration in the cerberus service db.
func saveServiceCfg(config SvcConfig) error {
	if config.Name == "" {
		return newError(ErrSaveServiceCfg, "empty service name is not allowed")
	}

	// Save scm properties
	if err := updateSCMProperties(&config); err != nil {
		return err
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

	if config.RecoveryActions != nil {
		var buf bytes.Buffer
		if err := gob.NewEncoder(&buf).Encode(config.RecoveryActions); err != nil {
			return newErrorW(ErrSaveServiceCfg, "failed to serialize recovery actions", err)
		}

		if err := key.SetBinaryValue("RecoveryActions", buf.Bytes()); err != nil {
			return newErrorW(ErrSaveServiceCfg, "failed to set recovery actions", err)
		}
	}

	return nil
}

func trimArgs(args []string) {
	if len(args) > 0 {
		DebugLogger.Println("Removing leading/trailing quotes from arguments...")
		for j := range args {
			n := len(args[j]) - 1
			if args[j][0] == '\'' && args[j][n] == '\'' {
				args[j] = args[j][1:n]
			}
		}
	}
}
