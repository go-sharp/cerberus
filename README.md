# Cerberus
 [![GoDoc](https://godoc.org/github.com/go-sharp/cerberus?status.svg)](https://godoc.org/github.com/go-sharp/cerberus)  
Cerberus is a Windows service helper program inspired by [NSSM](https://nssm.cc/).
It can be used to create and manage Windows services for ordinary binaries.

## Usage
```bash
Help Options:
  -h, --help  Show this help message

Available commands:
  edit      Editing an installed service
  install   Install a binary as service
  list      Show cerberus installed services
  recovery  Editing recovery actions for an installed service
  remove    Removes an installed service
  run       Runs a configured service
  version   Show version
```

### Install
```bash
Usage:
  cerberus_64.exe [OPTIONS] install [install-OPTIONS]

Install a binary as service

Help Options:
  -h, --help              Show this help message

[install command options]
      -v, --verbose       Verbose output
      -x, --executable=   Full path to the executable
      -w, --workdir=      Working directory of the executable, if not specified
                          the folder of the executable is used.
      -n, --name=         Name of the service, if not specified name of the
                          executable is used.
      -i, --display-name= Display name of the service, if not specified name of
                          the executable is used.
      -d, --desc=         Description of the service
      -a, --arg=          Arguments to pass to the executable in the same order
                          as specified. (ex. -a "-la" -a "123")
      -e, --env=          Environment variables to set for the executable. (ex.
                          -e "TERM=bash" -e "EDITOR=none")
```
### Edit
```bash
Usage:
  cerberus_64.exe [OPTIONS] edit [edit-OPTIONS] SERVICE_NAME

Editing an installed service

Help Options:
  -h, --help                    Show this help message

[edit command options]
      -v, --verbose             Verbose output
      -w, --workdir=            Working directory of the executable..
      -i, --display-name=       Display name of the service.
      -d, --desc=               Description of the service
      -a, --arg=                Arguments to pass to the executable in the same
                                order as specified. (ex. -a "-la" -a "123")
      -e, --env=                Environment variables to set for the
                                executable. (ex. -e "TERM=bash" -e
                                "EDITOR=none")
      -n, --dependencies=       Services on which this service depend on. (ex.
                                -a serviceA -a serviceB)
      -u, --user=               User under which this service will run.
      -p, --password=           Password for the specified service user.
      -s, --start-type=         Service start type. One of
                                [manual|autostart|delayed|disabled]
          --signal-ctrlc        Send Ctrl-C to process if service has to stop.
          --signal-wmquit       Send WM_QUIT to process if service has to stop.
          --signal-wmclose      Send WM_CLOSE to process if service has to stop.
          --no-signal           Restore default behaviour and doesn't send any
                                signals.
          --no-deps             Remove all dependencies for this service.
          --no-args             Remove all arguments for this service.
          --no-env              Remove all environment variables for this
                                service.
          --use-system-account  Use local system account to run this service.

[edit command arguments]
  SERVICE_NAME:                 Name of the service to edit.
```
### Remove
```bash
Usage:
  cerberus_64.exe [OPTIONS] remove [remove-OPTIONS] SERVICE_NAME

Removes an installed service

Help Options:
  -h, --help              Show this help message

[remove command options]
      -v, --verbose       Verbose output

[remove command arguments]
  SERVICE_NAME:           Name of the service to remove.
```
### Recovery Actions
```bash
Usage:
  cerberus_64.exe [OPTIONS] recovery set [set-OPTIONS] SERVICE_NAME ARGUMENTS...

Set a recovery action for an installed service

Help Options:
  -h, --help              Show this help message

[set command options]
      -v, --verbose       Verbose output
      -e, --exit-code=    Exit code to handle by this action.
      -a, --action=       Action to take if an error occurred. One of
                          [run-restart|none|restart|run]
      -d, --delay=        Delay restart of the program in seconds. (default: 0)
      -r, --max-restart=  Maximum restarts of the service within the specified
                          time span. Zero means unlimited restarts. (default: 0)
      -c, --reset-timer=  Specify the duration in seconds after which the
                          restart counter will be cleared. (default: 0)
      -x, --exec=         Specify the program to run if an error occurred.

[set command arguments]
  SERVICE_NAME:           Name of the service to set a recovery action.
  ARGUMENTS:              Arguments for the program to run if an error
                          occurred. Use '--' after SERVICE_NAME to specify
                          arguments starting with '-'.

Usage:
  cerberus_64.exe [OPTIONS] recovery del [del-OPTIONS] SERVICE_NAME EXIT_CODE

Deletes a recovery action for an installed service

Help Options:
  -h, --help              Show this help message

[del command options]
      -v, --verbose       Verbose output

[del command arguments]
  SERVICE_NAME:           Name of the service to delete a recovery action.
  EXIT_CODE:              Exit code for which the recovery action should be
                          deleted.                    
```

## Example
This is a minimal example:
```bash
cerberus_64.exe install -x "C:\Windows\notepad.exe" -n "MySuperService"
```
> Caveat: The *cerberus_64.exe* must not be moved after installation of a service, otherwise the service won't work anymore.

## Build
Requirments:
- Go >= 1.13 [https://golang.org/](https://golang.org/)
- GoVersionInfo [https://github.com/josephspurrier/goversioninfo](https://github.com/josephspurrier/goversioninfo)

Change into the *cmd* directory and run the following command:
```bash
C:\repo\cerberus\cmd> build.bat
```
