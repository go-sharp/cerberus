//go:generate goversioninfo -icon=cerberus.ico

package main

import (
	"fmt"
	"io"
	"log"
	"os"
	"strings"
	"time"

	"github.com/go-sharp/cerberus"
	"github.com/jessevdk/go-flags"
)

var version = "0.0.1-dev"

var installCommand InstallCommand
var runCommand RunCommand
var removeCommand RemoveCommand
var listCommand ListCommand
var parser = flags.NewParser(nil, flags.Default)

var startTypeMapping = map[cerberus.StartType]string{
	cerberus.ManualStartType:      "manual",
	cerberus.AutoStartType:        "autostart",
	cerberus.AutoDelayedStartType: "delayed autostart",
	cerberus.DisabledStartType:    "disabled",
}

var writer io.Writer = os.Stdout

func init() {
	parser.AddCommand("version", "Show version", "Show version", CommandFunc(showVersion))
	parser.AddCommand("list", "Show cerberus installed services", "Show cerberus installed services", &listCommand)
	parser.AddCommand("install", "Install a binary as service", "Install a binary as service", &installCommand)
	parser.AddCommand("run", "Runs a configured service", "Runs a configured service", &runCommand)
	parser.AddCommand("remove", "Removes an installed service", "Removes an installed service", &removeCommand)
	recCmd, _ := parser.AddCommand("recovery",
		"Editing recovery actions for an installed service",
		"Editing recovery actions for an installed service",
		CommandFunc(nil))
	recCmd.AddCommand("set", "Sets a recovery action for an installed service", "Set a recovery action for an installed service", &RecoverySetCommand{})
	recCmd.AddCommand("del", "Deletes a recovery action for an installed service", "Deletes a recovery action for an installed service", &RecoveryDelCommand{})

	parser.AddCommand("edit", "Editing an installed service", "Editing an installed service", &EditCommand{})

	// Enable logging to a file, required to debug service errors while executing the run command.
	logpath := os.Getenv("CERBERUS_LOGGER")
	if logpath != "" {
		fs, err := os.OpenFile(logpath, os.O_CREATE|os.O_APPEND, 0666)
		if err != nil {
			cerberus.Logger.Fatalln(err)
		}
		writer = io.MultiWriter(fs, os.Stdout)
		cerberus.Logger = log.New(writer, "Cerberus: ", 0)
	}
}

func main() {
	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func showVersion(args []string) error {
	fmt.Println("Cerberus: ", version)
	return nil
}

// RootCommand used for all subcommands
type RootCommand struct {
	Verbose bool `long:"verbose" short:"v" description:"Verbose output"`
}

// Execute will setup root command properly. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (r *RootCommand) Execute(args []string) (err error) {
	_, verbose := os.LookupEnv("CERBERUS_VERBOSE")
	if r.Verbose || verbose {
		cerberus.DebugLogger.SetOutput(writer)
	}

	return nil
}

// ListCommand shows all cerberus installed services.
type ListCommand struct {
	RootCommand
	Query string `long:"filter" short:"f" description:"Only show services whose name contains the filter word."`
}

// Execute will list all with cerberus installed services. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (r *ListCommand) Execute(args []string) (err error) {
	if err := r.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	svcs, err := cerberus.LoadServicesCfg()
	if err != nil {
		cerberus.DebugLogger.Fatalln(err)
	}

	fmt.Printf("\nCerberus installed services:\n")
	fmt.Println(strings.Repeat("-", 80))

	p := keyValuePrinter{indentSize: 5}
	for _, s := range svcs {
		if r.Query != "" {
			if !strings.Contains(strings.ToLower(s.Name), strings.ToLower(r.Query)) {
				continue
			}
		}

		p.println("Name", s.Name)
		p.println("Display Name", s.DisplayName)
		p.println("Description", s.Desc)
		p.println("Executable Path", s.ExePath)
		p.println("Working Directory", s.WorkDir)
		if len(s.Args) > 0 {
			p.println("Arguments", strings.Join(s.Args, " "))
		}
		if len(s.Env) > 0 {
			p.println("Environment Variables", strings.Join(s.Env, " "))
		}
		p.println("Start Type", startTypeMapping[s.StartType])
		if s.StopSignal != cerberus.NoSignal {
			p.println("Stop Signal", s.StopSignal)
		}
		p.println("Service User", s.ServiceUser)
		if len(s.Dependencies) > 0 {
			p.println("Dependencies", strings.Join(s.Dependencies, " | "))
		}
		var actlng = len(s.RecoveryActions)
		if actlng > 0 {
			p.println("Recovery Actions", "")
			p.indent()
			for _, action := range s.RecoveryActions {
				p.println("Error Code", action.ExitCode)
				p.println("Action", mapAction(action.Action))
				if action.Action&cerberus.RestartAction == cerberus.RestartAction {
					p.println("Delay", action.Delay)
					p.println("Max Restarts", action.MaxRestarts)
					p.println("Reset After", action.ResetAfter)
				}
				if action.Action&cerberus.RunProgramAction == cerberus.RunProgramAction {
					p.println("Program", action.Program)
					p.println("Arguments", fmt.Sprintf("[%v]", concatArgs(action.Arguments)))
				}
				if actlng > 1 {
					p.println("-", nil)
				}
				actlng--
			}
		}

		p.writeTo(os.Stdout)
		fmt.Fprintf(os.Stdout, "%v\n", strings.Repeat("-", 80))
	}

	return nil
}

// InstallCommand used to install a binary as service.
type InstallCommand struct {
	RootCommand
	ExePath     string   `long:"executable" short:"x" description:"Full path to the executable" required:"true"`
	WorkDir     string   `long:"workdir" short:"w" description:"Working directory of the executable, if not specified the folder of the executable is used."`
	Name        string   `long:"name" short:"n" description:"Name of the service, if not specified name of the executable is used."`
	DisplayName string   `long:"display-name" short:"i" description:"Display name of the service, if not specified name of the executable is used."`
	Desc        string   `long:"desc" short:"d" description:"Description of the service"`
	Args        []string `long:"arg" short:"a" description:"Arguments to pass to the executable in the same order as specified. (ex. -a \"-la\" -a \"123\")"`
	Env         []string `long:"env" short:"e" description:"Environment variables to set for the executable. (ex. -e \"TERM=bash\" -e \"EDITOR=none\")"`
}

// Execute will install a binary as service. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (i *InstallCommand) Execute(args []string) (err error) {
	if err := i.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	svcCfg := cerberus.SvcConfig{
		ExePath:     i.ExePath,
		Name:        i.Name,
		WorkDir:     i.WorkDir,
		Args:        i.Args,
		Env:         i.Env,
		Desc:        i.Desc,
		DisplayName: i.DisplayName,
	}

	if err := cerberus.InstallService(svcCfg); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	return nil
}

// RemoveCommand used to remove a service.
type RemoveCommand struct {
	RootCommand
	Args struct {
		Name string `positional-arg-name:"SERVICE_NAME" description:"Name of the service to remove." required:"yes"`
	} `positional-args:"yes" required:"1"`
}

// Execute will remove an installed service. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (r *RemoveCommand) Execute(args []string) error {
	if err := r.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	if err := cerberus.RemoveService(r.Args.Name); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	return nil
}

// RunCommand runs the configured service directly.
type RunCommand struct {
	RootCommand
	Args struct {
		Name string `positional-arg-name:"SERVICE_NAME" description:"Name of the service to run."`
	} `positional-args:"yes" required:"1"`
}

// Execute will run the service handler.
func (r *RunCommand) Execute(args []string) (err error) {
	// If we run as a service, we need to catch panics.
	defer func() {
		if r := recover(); r != nil {
			cerberus.Logger.Fatalln(r)
		}
	}()

	if err := r.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	if err := cerberus.RunService(r.Args.Name); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	return nil
}

// EditCommand runs the configured service directly.
type EditCommand struct {
	RootCommand
	WorkDir      *string   `long:"workdir" short:"w" description:"Working directory of the executable.."`
	DisplayName  *string   `long:"display-name" short:"i" description:"Display name of the service."`
	Desc         *string   `long:"desc" short:"d" description:"Description of the service"`
	Arguments    *[]string `long:"arg" short:"a" description:"Arguments to pass to the executable in the same order as specified. (ex. -a \"-la\" -a \"123\")"`
	Env          *[]string `long:"env" short:"e" description:"Environment variables to set for the executable. (ex. -e \"TERM=bash\" -e \"EDITOR=none\")"`
	Dependencies *[]string `long:"dependencies" short:"n" description:"Services on which this service depend on. (ex. -a serviceA -a serviceB)"`
	ServiceUser  *string   `long:"user" short:"u" description:"User under which this service will run."`
	Password     *string   `long:"password" short:"p" description:"Password for the specified service user."`
	StartType    *string   `long:"start-type" short:"s" description:"Service start type. One of [manual|autostart|delayed|disabled]"`
	// Flags
	SignalCtrlC    *bool `long:"signal-ctrlc" description:"Send Ctrl-C to process if service has to stop."`
	SignalWmQuit   *bool `long:"signal-wmquit" description:"Send WM_QUIT to process if service has to stop."`
	SignalWmClose  *bool `long:"signal-wmclose" description:"Send WM_CLOSE to process if service has to stop."`
	NoSignal       *bool `long:"no-signal" description:"Restore default behaviour and doesn't send any signals."`
	NoDependencies *bool `long:"no-deps" description:"Remove all dependencies for this service."`
	NoArgs         *bool `long:"no-args" description:"Remove all arguments for this service."`
	NoEnv          *bool `long:"no-env" description:"Remove all environment variables for this service."`
	UseLocalSystem *bool `long:"use-system-account" description:"Use local system account to run this service."`
	Args           struct {
		Name string `positional-arg-name:"SERVICE_NAME" description:"Name of the service to edit."`
	} `positional-args:"yes" required:"1"`
}

// Execute will run the service handler.
func (e *EditCommand) Execute(args []string) (err error) {
	if err := e.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	svc, err := cerberus.LoadServiceCfg(e.Args.Name)
	if err != nil {
		cerberus.Logger.Fatalln(err)
	}

	if e.WorkDir != nil && *e.WorkDir != "" {
		svc.WorkDir = *e.WorkDir
	}

	if e.DisplayName != nil {
		svc.DisplayName = *e.DisplayName
	}

	if e.Desc != nil {
		svc.Desc = *e.Desc
	}

	if e.Arguments != nil {
		svc.Args = *e.Arguments
	}

	if e.Env != nil {
		svc.Env = *e.Env
	}

	if e.Dependencies != nil {
		svc.Dependencies = *e.Dependencies
	}

	if e.ServiceUser != nil {
		svc.ServiceUser = *e.ServiceUser
	}

	if e.Password != nil {
		svc.Password = e.Password
	}

	if e.StartType != nil {
		switch *e.StartType {
		case "manual":
			svc.StartType = cerberus.ManualStartType
		case "autostart":
			svc.StartType = cerberus.AutoStartType
		case "delayed":
			svc.StartType = cerberus.AutoDelayedStartType
		case "disabled":
			svc.StartType = cerberus.DisabledStartType
		default:
			cerberus.Logger.Fatalln("Invalid start type passed: one of (manual|autostart|delayed|disabled) is required.")
		}
	}

	if e.NoSignal != nil && *e.NoSignal {
		svc.StopSignal = cerberus.NoSignal
	}

	if e.SignalCtrlC != nil && *e.SignalCtrlC {
		svc.StopSignal = svc.StopSignal | cerberus.CtrlCSignal
	}

	if e.SignalWmClose != nil && *e.SignalWmClose {
		svc.StopSignal = svc.StopSignal | cerberus.WmCloseSignal
	}

	if e.SignalWmQuit != nil && *e.SignalWmQuit {
		svc.StopSignal = svc.StopSignal | cerberus.WmQuitSignal
	}

	if e.NoArgs != nil && *e.NoArgs {
		svc.Args = []string{}
	}

	if e.NoEnv != nil && *e.NoEnv {
		svc.Env = []string{}
	}

	if e.NoDependencies != nil && *e.NoDependencies {
		svc.Dependencies = []string{}
	}

	if e.UseLocalSystem != nil && *e.UseLocalSystem {
		svc.ServiceUser = "LocalSystem"
	}

	if err := cerberus.UpdateService(*svc); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	return nil
}

// RecoveryDelCommand delete a recovery action for an installed service..
type RecoveryDelCommand struct {
	RootCommand
	Args struct {
		Name     string `positional-arg-name:"SERVICE_NAME" description:"Name of the service to delete a recovery action."`
		ExitCode int    `positional-arg-name:"EXIT_CODE" description:"Exit code for which the recovery action should be deleted."`
	} `positional-args:"yes" required:"2"`
}

// Execute will run the service handler.
func (r *RecoveryDelCommand) Execute(args []string) (err error) {
	if err := r.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	svc, err := cerberus.LoadServiceCfg(r.Args.Name)
	if err != nil {
		cerberus.Logger.Fatalln(err)
	}

	if _, ok := svc.RecoveryActions[r.Args.ExitCode]; ok {
		delete(svc.RecoveryActions, r.Args.ExitCode)
	}

	err = cerberus.UpdateService(*svc)
	if err != nil {
		cerberus.Logger.Fatalln(err)
	}

	return nil
}

// RecoverySetCommand sets a recovery action for an installed service..
type RecoverySetCommand struct {
	RootCommand
	ExitCode    int    `long:"exit-code" short:"e" description:"Exit code to handle by this action." required:"yes"`
	Action      string `long:"action" short:"a" description:"Action to take if an error occurred. One of [run-restart|none|restart|run]" required:"yes"`
	Delay       int    `long:"delay" short:"d" description:"Delay restart of the program in seconds." default:"0"`
	MaxRestarts int    `long:"max-restart" short:"r" description:"Maximum restarts of the service within the specified time span. Zero means unlimited restarts." default:"0"`
	ResetAfter  int    `long:"reset-timer" short:"c" description:"Specify the duration in seconds after which the restart counter will be cleared." default:"0"`
	Program     string `long:"exec" short:"x" description:"Specify the program to run if an error occurred."`
	Args        struct {
		Name      string   `positional-arg-name:"SERVICE_NAME" description:"Name of the service to set a recovery action."`
		Arguments []string `positional-arg-name:"ARGUMENTS" description:"Arguments for the program to run if an error occurred. Use '--' after SERVICE_NAME to specify arguments starting with '-'."`
	} `positional-args:"yes" required:"1"`
}

// Execute will run the service handler.
func (r *RecoverySetCommand) Execute(args []string) (err error) {
	if err := r.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	svc, err := cerberus.LoadServiceCfg(r.Args.Name)
	if err != nil {
		cerberus.Logger.Fatalln(err)
	}

	action := cerberus.SvcRecoveryAction{
		ExitCode:    r.ExitCode,
		Arguments:   r.Args.Arguments,
		Delay:       r.Delay,
		MaxRestarts: r.MaxRestarts,
		ResetAfter:  time.Second * time.Duration(r.ResetAfter),
		Program:     r.Program,
	}

	switch r.Action {
	case "none":
		action.Action = cerberus.NoAction
	case "run":
		action.Action = cerberus.RunProgramAction
	case "restart":
		action.Action = cerberus.RestartAction
	case "run-restart":
		action.Action = cerberus.RunAndRestartAction
	default:
		cerberus.Logger.Fatalln("Invalid recovery action passed: one of (run|restart|none|run-restart) is required.")
	}

	svc.RecoveryActions[action.ExitCode] = action

	if err := cerberus.UpdateService(*svc); err != nil {
		cerberus.Logger.Fatalln(err)
	}

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
	if c.fn != nil {
		return c.fn(args)
	}
	return nil
}

func concatArgs(args []string) string {
	for i := range args {
		args[i] = fmt.Sprintf("\"%v\"", args[i])
	}

	return strings.Join(args, " ")
}

func mapAction(action cerberus.RecoveryAction) string {
	switch action {
	case cerberus.NoAction:
		return "none"
	case cerberus.RunProgramAction:
		return "run"
	case cerberus.RestartAction:
		return "restart"
	case cerberus.RunAndRestartAction:
		return "run-restart"
	default:
		return ""
	}
}
