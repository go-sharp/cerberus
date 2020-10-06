package main

import (
	"fmt"
	"os"
	"strings"
	"text/tabwriter"

	"github.com/go-sharp/cerberus"
	"github.com/jessevdk/go-flags"
)

const version = "2.0.0"

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

func (r RootCommand) logDebug(format string, a ...interface{}) {
	if r.Verbose {
		r.log(format, a...)
	}
}

func (r RootCommand) log(format string, a ...interface{}) {
	fmt.Printf("Cerberus: "+format+"\n", a...)
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

	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	fmt.Fprintf(w, "\nName\tDisplay Name\tDescription\tWorking Dir\tArgs\tEnvironment\n")
	for _, s := range svcs {
		fmt.Fprintf(w, "%v\t%v\t%v\t%v\t%v\t%v\n", s.Name, s.DisplayName, s.Desc, s.WorkDir, strings.Join(s.Args, " "), strings.Join(s.Env, " "))
	}
	w.Flush()

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
	Name string `long:"name" short:"n" description:"Try to remove service by name" required:"yes"`
}

// Execute will remove an installed service. The args parameter is not used
// and is only to fullfil the go-flags commander interface.
func (r *RemoveCommand) Execute(args []string) error {
	if err := r.RootCommand.Execute(args); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	if err := cerberus.RemoveService(r.Name); err != nil {
		cerberus.Logger.Fatalln(err)
	}

	return nil
}

// RunCommand runs the configured service directly.
type RunCommand struct {
	RootCommand
	Args struct {
		Name string `positional-arg-name:"SERVICE_NAME" description:"Name of the service to remove."`
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
