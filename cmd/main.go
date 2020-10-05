package main

import (
	"fmt"
	"os"

	"github.com/go-sharp/cerberus"
	"github.com/jessevdk/go-flags"
)

const version = "1.0.1"

var installCommand cerberus.InstallCommand
var runCommand cerberus.RunCommand
var removeCommand cerberus.RemoveCommand
var parser = flags.NewParser(nil, flags.Default)

func init() {
	parser.AddCommand("version", "Show version", "Show version", cerberus.CommandFunc(showVersion))
	parser.AddCommand("install", "Install a binary as service", "Install a binary as service", &installCommand)
	parser.AddCommand("run", "Runs the configured binary", "Runs the configured binary", &runCommand)
	parser.AddCommand("remove", "Removes an installed service (Will read service from config file)", "Removes an installed service (Will read service from config file)", &removeCommand)
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
