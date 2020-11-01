package main

import (
	"fmt"
	"os"
	"strings"
	"time"

	"github.com/go-sharp/cerberus"
	"github.com/jessevdk/go-flags"
)

const version = "2.1.0"

var installCommand InstallCommand
var runCommand RunCommand
var removeCommand RemoveCommand
var listCommand ListCommand
var parser = flags.NewParser(nil, flags.Default)

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

}

func main() {

	_, err := parser.Parse()
	if err != nil {
		os.Exit(1)
	}
	os.Exit(0)
}

func showVersion(args []string) error {
	fmt.Println("Cerberus: Version ", version)
	return nil
}

// RootCommand used for all subcommands
type RootCommand struct {
	Verbose bool `long:"verbose" short:"v" description:"Verbose output"`
}

// Execute will setup root command properly. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (r *RootCommand) Execute(args []string) (err error) {
	if r.Verbose {
		cerberus.DebugLogger.SetOutput(os.Stdout)
	}

	return nil
}

// ListCommand shows all cerberus installed services.
type ListCommand struct {
	RootCommand
}

// Execute will list all with cerberus installed services. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (r *ListCommand) Execute(args []string) (err error) {
	if err := r.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	if r.Verbose {
		cerberus.DebugLogger.SetOutput(os.Stdout)
	}

	svcs, err := cerberus.LoadServicesCfg()
	if err != nil {
		cerberus.DebugLogger.Fatalln(err)
	}

	fmt.Printf("\nCerberus installed services:\n")
	fmt.Println(strings.Repeat("-", 80))

	firstlvl := 22
	secondlvl := firstlvl + 8
	for _, s := range svcs {
		fmt.Fprintln(os.Stdout, fill("Name:", firstlvl), s.Name)
		fmt.Fprintln(os.Stdout, fill("Display Name:", firstlvl), s.DisplayName)
		fmt.Fprintln(os.Stdout, fill("Description:", firstlvl), s.Desc)
		fmt.Fprintln(os.Stdout, fill("Executable Path:", firstlvl), s.ExePath)
		fmt.Fprintln(os.Stdout, fill("Working Directory:", firstlvl), s.WorkDir)
		fmt.Fprintln(os.Stdout, fill("Arguments:", firstlvl), strings.Join(s.Args, " "))
		fmt.Fprintln(os.Stdout, fill("Environment Variables:", firstlvl), strings.Join(s.Env, " "))
		fmt.Fprintln(os.Stdout, fill("Recovery Actions:", firstlvl))

		var actlng = len(s.RecoveryActions)
		for _, action := range s.RecoveryActions {
			fmt.Fprintln(os.Stdout, fill("Action:", secondlvl), mapAction(action.Action))
			fmt.Fprintln(os.Stdout, fill("Error Code:", secondlvl), action.ExitCode)
			if action.Action&cerberus.RestartAction == cerberus.RestartAction {
				fmt.Fprintln(os.Stdout, fill("Delay:", secondlvl), action.Delay)
				fmt.Fprintln(os.Stdout, fill("Max Restarts:", secondlvl), action.MaxRestarts)
				fmt.Fprintln(os.Stdout, fill("Reset After:", secondlvl), action.ResetAfter)
			}
			if action.Action&cerberus.RunProgramAction == cerberus.RunProgramAction {
				fmt.Fprintln(os.Stdout, fill("Program:", secondlvl), action.Program)
				fmt.Fprintln(os.Stdout, fill("Arguments:", secondlvl), fmt.Sprintf("[%v]", concatArgs(action.Arguments)))
			}
			if actlng > 1 {
				fmt.Fprintln(os.Stdout, fill("-", secondlvl))
			}
			actlng--
		}
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
	Env         []string `long:"env" short:"e" description:"Arguments to pass to the executable in the same order as specified. (ex. -a \"-la\" -a \"123\")"`
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
	if err := r.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	if err := cerberus.RunService(r.Args.Name); err != nil {
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
	Action      string `long:"action" short:"a" description:"Action to take if an error occured." choice:"run-restart" choice:"none" choice:"restart" choice:"run" required:"yes"`
	Delay       int    `long:"delay" short:"d" description:"Delay restart of the program in seconds." default:"0"`
	MaxRestarts int    `long:"max-restart" short:"r" description:"Maximum restarts of the service within the specified time span. Zero means unlimited restarts." default:"0"`
	ResetAfter  int    `long:"reset-timer" short:"c" description:"Specify the duration in seconds after which the restart counter will be cleared." default:"0"`
	Program     string `long:"exec" short:"x" description:"Specify the program to run if an error occured."`
	Args        struct {
		Name      string   `positional-arg-name:"SERVICE_NAME" description:"Name of the service to set a recovery action."`
		Arguments []string `positional-arg-name:"ARGUMENTS" description:"Arguments for the program to run if an error occured. Use '--' after SERVICE_NAME to specify arguments starting with '-'."`
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

func fill(s string, min int) string {
	n := min - len(s)
	if n < 0 {
		return s
	}

	return strings.Repeat(" ", n) + s
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
