package handler

import (
	"bufio"
	"encoding/json"
	"fmt"
	"io"
	"os"
	"sync"

	"github.com/rackspace/rack/auth"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud"
	"github.com/rackspace/rack/output"
)

// StreamPipeHandler is an interface that commands implement if they can stream input
// from STDIN.
type StreamPipeHandler interface {
	// PipeHandler is an interface that commands implement if they can accept input
	// from STDIN.
	PipeHandler
	// StreamField is the field that the command accepts for streaming input on STDIN.
	StreamField() string
	// HandleStreamPipe is a method that commands implement for processing streaming, piped input.
	HandleStreamPipe(*Resource) error
}

type StatusUpdater interface {
	Commander
	StatusChannel(*Resource) chan interface{}
}

// PipeHandler is an interface that commands implement if they can accept input
// from STDIN.
type PipeHandler interface {
	// Commander is an interface that all commands will implement.
	Commander
	// HandleSingle contains logic for processing a single resource. This method
	// will be used if input isn't sent to STDIN, so it will contain, for example,
	// logic for handling flags that would be mandatory if otherwise not piped in.
	HandleSingle(*Resource) error
	// HandlePipe is a method that commands implement for processing piped input.
	HandlePipe(*Resource, string) error
	// StdinField is the field that the command accepts on STDIN.
	StdinField() string
}

// PreJSONer is an interface that commands will satisfy if they have a `PreJSON` method.
type PreJSONer interface {
	PreJSON(*Resource) error
}

// PreCSVer is an interface that commands will satisfy if they have a `PreCSV` method.
type PreCSVer interface {
	PreCSV(*Resource) error
}

// PreTabler is an interface that commands will satisfy if they have a `PreTable` method.
type PreTabler interface {
	PreTable(*Resource) error
}

// Commander is an interface that all commands implement.
type Commander interface {
	// See `Context`.
	Context() *Context
	// Keys returns the keys available for the command output.
	Keys() []string
	// ServiceClientType returns the type of the service client to use.
	ServiceClientType() string
	// HandleFlags processes flags for the command that are relevant for both piped
	// and non-piped commands.
	HandleFlags(*Resource) error
	// Execute executes the command's HTTP request.
	Execute(*Resource)
}

// Handle is the function that handles all commands. It accepts a Commander as
// a parameter, which all commands implement.
func Handle(command Commander) {
	ctx := command.Context()
	ctx.ServiceClientType = command.ServiceClientType()
	ctx.Results = make(chan *Resource)

	resource := &Resource{
		Keys: command.Keys(),
	}

	err := ctx.CheckArgNum(0)
	if err != nil {
		resource.Err = err
		errExit1(command, resource)
	}

	err = ctx.handleGlobalOptions()
	if err != nil {
		resource.Err = err
		errExit1(command, resource)
	}

	client, err := auth.NewClient(ctx.CLIContext, ctx.ServiceClientType, ctx.logger, ctx.GlobalOptions.noCache, ctx.GlobalOptions.useServiceNet)
	if err != nil {
		resource.Err = err
		errExit1(command, resource)
	}
	client.HTTPClient.Transport.(*auth.LogRoundTripper).Logger = ctx.logger
	ctx.ServiceClient = client

	err = command.HandleFlags(resource)
	if err != nil {
		resource.Err = err
		errExit1(command, resource)
	}

	handleExecute(command, resource)

	if statusUpdater, ok := command.(StatusUpdater); ok {
		statusChannel := statusUpdater.StatusChannel(resource)
		if err != nil {
			panic(err)
		}
		for msg := range statusChannel {
			fmt.Println(msg)
		}
	}

	for resource := range ctx.Results {
		processResult(command, resource)
		printResult(command, resource)
	}

	ctx.storeCredentials()
}

func handleExecute(command Commander, resource *Resource) {
	ctx := command.Context()
	// can the command accept input on STDIN?
	if pipeableCommand, ok := command.(PipeHandler); ok {
		// should we expect something on STDIN?
		if ctx.CLIContext.IsSet("stdin") {
			stdinField := ctx.CLIContext.String("stdin")
			// if so, does the given field accept pipeable input?
			if stdinField == pipeableCommand.StdinField() {
				wg := sync.WaitGroup{}
				scanner := bufio.NewScanner(os.Stdin)
				for scanner.Scan() {
					item := scanner.Text()
					wg.Add(1)
					go func(resource Resource) {
						err := pipeableCommand.HandlePipe(&resource, item)
						if err != nil {
							resource.Err = fmt.Errorf("Error handling pipeable command on %s: %s\n", item, err)
							ctx.Results <- &resource
						} else {
							pipeableCommand.Execute(&resource)
							ctx.Results <- &resource
						}
						wg.Done()
					}(*resource)
				}
				if scanner.Err() != nil {
					resource.Err = scanner.Err()
					errExit1(command, resource)
				}
				wg.Wait()
				close(ctx.Results)
				// else, does the given command and field accept streaming input?
			} else if streamPipeableCommand, ok := pipeableCommand.(StreamPipeHandler); ok && streamPipeableCommand.StreamField() == stdinField {
				go func(resource Resource) {
					err := streamPipeableCommand.HandleStreamPipe(&resource)
					if err != nil {
						resource.Err = fmt.Errorf("Error handling streamable, pipeable command: %s\n", err)
					} else {
						streamPipeableCommand.Execute(&resource)
					}
					ctx.Results <- &resource
					close(ctx.Results)
				}(*resource)
			} else {
				// the value provided to the `stdin` flag is not valid
				resource.Err = fmt.Errorf("Unknown STDIN field: %s\n", stdinField)
				errExit1(command, resource)
			}
			// since no `stdin` flag was provided, treat as a singular execution
		} else {
			go func(resource Resource) {
				err := pipeableCommand.HandleSingle(&resource)
				if err != nil {
					resource.Err = err
					errExit1(command, &resource)
				}
				command.Execute(&resource)
				ctx.Results <- &resource
				close(ctx.Results)
			}(*resource)
		}
		// the command is a single execution (as opposed to reading from a pipe)
	} else {
		go func(resource Resource) {
			command.Execute(&resource)
			ctx.Results <- &resource
			close(ctx.Results)
		}(*resource)
	}
}

func processResult(command Commander, resource *Resource) {
	ctx := command.Context()

	// if an error was encountered during `handleExecution`, return it instead of
	// the `resource.Result`.
	if resource.Err != nil {
		ctx.CLIContext.App.Writer = os.Stderr
		resource.Keys = []string{"error"}
		var errorBody string

		switch resource.Err.(type) {
		case *gophercloud.UnexpectedResponseCodeError:
			errBodyRaw := resource.Err.(*gophercloud.UnexpectedResponseCodeError).Body
			errMap := make(map[string]map[string]interface{})
			err := json.Unmarshal(errBodyRaw, &errMap)
			if err != nil {
				errorBody = string(errBodyRaw)
				break
			}
			for _, v := range errMap {
				errorBody = v["message"].(string)
				break
			}
		default:
			errorBody = resource.Err.Error()
		}

		resource.Result = map[string]interface{}{"error": errorBody}
	} else if resource.Result == nil {
		switch resource.Result.(type) {
		case []map[string]interface{}:
			resource.Result = fmt.Sprintf("No results found\n")
		default:
			resource.Result = fmt.Sprintf("No result found.\n")
		}
	} else {
		// limit the returned fields if any were given in the `fields` flag
		ctx.limitFields(resource)

		var err error
		// apply any output-specific transformations on the result
		switch ctx.GlobalOptions.output {
		case "json":
			if jsoner, ok := command.(PreJSONer); ok {
				err = jsoner.PreJSON(resource)
			}
		case "csv":
			if csver, ok := command.(PreCSVer); ok {
				err = csver.PreCSV(resource)
			}
		default:
			if tabler, ok := command.(PreTabler); ok {
				err = tabler.PreTable(resource)
			}
		}
		if err != nil {
			resource.Keys = []string{"error"}
			resource.Result = map[string]interface{}{"error": err.Error()}
		}
	}
}

func printResult(command Commander, resource *Resource) {
	ctx := command.Context()
	w := ctx.CLIContext.App.Writer
	keys := resource.Keys
	noHeader := false
	if ctx.GlobalOptions.noHeader {
		noHeader = true
	}
	switch resource.Result.(type) {
	case map[string]interface{}:
		m := resource.Result.(map[string]interface{})
		m = onlyNonNil(m)
		switch ctx.GlobalOptions.output {
		case "json":
			output.MetadataJSON(w, m, keys)
		case "csv":
			output.MetadataCSV(w, m, keys, noHeader)
		default:
			output.MetadataTable(w, m, keys)
		}
	case []map[string]interface{}:
		ms := resource.Result.([]map[string]interface{})
		for i, m := range ms {
			ms[i] = onlyNonNil(m)
		}
		switch ctx.GlobalOptions.output {
		case "json":
			output.ListJSON(w, ms, keys)
		case "csv":
			output.ListCSV(w, ms, keys, noHeader)
		default:
			output.ListTable(w, ms, keys, noHeader)
		}
	case io.Reader:
		if _, ok := resource.Result.(io.ReadCloser); ok {
			defer resource.Result.(io.ReadCloser).Close()
		}
		_, err := io.Copy(w, resource.Result.(io.Reader))
		if err != nil {
			fmt.Fprintf(os.Stderr, "Error copying (io.Reader) result: %s\n", err)
		}
	default:
		switch ctx.GlobalOptions.output {
		case "json":
			output.DefaultJSON(w, resource.Result)
		default:
			fmt.Fprintf(w, "%v\n", resource.Result)
		}
	}
}

// errExit1 tells `rack` to print the error and exit.
func errExit1(command Commander, resource *Resource) {
	processResult(command, resource)
	printResult(command, resource)
	os.Exit(1)
}
