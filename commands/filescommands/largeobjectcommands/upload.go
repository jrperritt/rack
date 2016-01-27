package largeobjectcommands

import (
	"fmt"
	"io"
	"os"
	"runtime"
	"strings"
	"time"

	"github.com/rackspace/rack/commandoptions"
	"github.com/rackspace/rack/commands/filescommands/objectcommands"
	"github.com/rackspace/rack/handler"
	"github.com/rackspace/rack/internal/github.com/codegangsta/cli"
	"github.com/rackspace/rack/internal/github.com/dustin/go-humanize"
	"github.com/rackspace/rack/internal/github.com/gosuri/uiprogress"
	osObjects "github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/rackspace/rack/util"
)

var upload = cli.Command{
	Name:        "upload",
	Usage:       util.Usage(commandPrefix, "upload", "--container <containerName> --size-pieces <sizePieces> [--name <objectName> | --stdin file]"),
	Description: "Uploads a large object",
	Action:      actionUpload,
	Flags:       commandoptions.CommandFlags(flagsUpload, keysUpload),
	BashComplete: func(c *cli.Context) {
		commandoptions.CompleteFlags(commandoptions.CommandFlags(flagsUpload, keysUpload))
	},
}

func flagsUpload() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "container",
			Usage: "[required] The name of the container to upload the object into.",
		},
		cli.StringFlag{
			Name:  "name",
			Usage: "[optional; required if `stdin` isn't provided with value of 'file'] The name the object should have in the Cloud Files container.",
		},
		cli.StringFlag{
			Name:  "file",
			Usage: "[optional; required if `stdin` isn't provided] The file name containing the contents to upload.",
		},
		cli.StringFlag{
			Name: "stdin",
			Usage: strings.Join([]string{"[optional; required if `file` isn't provided] The field being piped to STDIN, if any.",
				"Valid values are: file, content. If 'file' is given, the names of the objects in the container will match",
				"the names on the user's file system."}, "\n\t"),
		},
		cli.IntFlag{
			Name:  "size-pieces",
			Usage: "[required] The size of the pieces (in MB) to divide the file into.",
		},
		cli.StringFlag{
			Name:  "content-type",
			Usage: "[optional] The Content-Type header.",
		},
		cli.IntFlag{
			Name:  "content-length",
			Usage: "[optional] The Content-Length header.",
		},
		/*
			cli.StringFlag{
				Name:  "metadata",
				Usage: "[optional] A comma-separated string of key=value pairs.",
			},
		*/
		cli.IntFlag{
			Name:  "concurrency",
			Usage: "[optional] The number of workers that will be uploading pieces at the same time.",
		},
		cli.BoolFlag{
			Name:  "quiet",
			Usage: "[optional] By default, progress bars will be outputted. If --quiet is provided, only a final summary will be outputted.",
		},
	}
}

var keysUpload = []string{}

type paramsUpload struct {
	container     string
	object        string
	stream        io.Reader
	opts          osObjects.CreateLargeOpts
	quiet         bool
	statusChannel chan interface{}
}

type commandUpload handler.Command

func actionUpload(c *cli.Context) {
	command := &commandUpload{
		Ctx: &handler.Context{
			CLIContext: c,
		},
	}
	handler.Handle(command)
}

func (command *commandUpload) Context() *handler.Context {
	return command.Ctx
}

func (command *commandUpload) Keys() []string {
	return keysUpload
}

func (command *commandUpload) ServiceClientType() string {
	return serviceClientType
}

func (command *commandUpload) HandleFlags(resource *handler.Resource) error {
	err := command.Ctx.CheckFlagsSet([]string{"container", "name", "size-pieces"})
	if err != nil {
		return err
	}

	c := command.Ctx.CLIContext
	containerName := c.String("container")

	if err := objectcommands.CheckContainerExists(command.Ctx.ServiceClient, containerName); err != nil {
		return err
	}

	opts := osObjects.CreateLargeOpts{
		CreateOpts: osObjects.CreateOpts{
			ContentLength: int64(c.Int("content-length")),
			ContentType:   c.String("content-type"),
		},
		SizePieces:  int64(c.Int("size-pieces")),
		Concurrency: c.Int("concurrency"),
	}

	/*
		if c.IsSet("metadata") {
			metadata, err := command.Ctx.CheckKVFlag("metadata")
			if err != nil {
				return err
			}
			opts.Metadata = metadata
		}
	*/

	resource.Params = &paramsUpload{
		container:     containerName,
		object:        c.String("name"),
		opts:          opts,
		quiet:         c.Bool("quiet"),
		statusChannel: make(chan interface{}),
	}

	return nil
}

func (command *commandUpload) HandlePipe(resource *handler.Resource, item string) error {
	file, err := os.Open(item)
	if err != nil {
		return err
	}
	resource.Params.(*paramsUpload).object = file.Name()

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	resource.Params.(*paramsUpload).opts.ContentLength = fileInfo.Size()

	resource.Params.(*paramsUpload).stream = file
	return nil
}

func (command *commandUpload) HandleSingle(resource *handler.Resource) error {
	err := command.Ctx.CheckFlagsSet([]string{"file", "name"})
	if err != nil {
		return err
	}
	resource.Params.(*paramsUpload).object = command.Ctx.CLIContext.String("name")

	file, err := os.Open(command.Ctx.CLIContext.String("file"))
	if err != nil {
		return err
	}

	fileInfo, err := file.Stat()
	if err != nil {
		return err
	}

	resource.Params.(*paramsUpload).opts.ContentLength = fileInfo.Size()

	resource.Params.(*paramsUpload).stream = file
	return nil
}

func (command *commandUpload) Execute(resource *handler.Resource) {
	params := resource.Params.(*paramsUpload)
	containerName := params.container
	objectName := params.object
	stream := params.stream
	opts := params.opts
	statusChannel := params.statusChannel

	gophercloudChannel := make(chan *osObjects.TransferStatus)
	opts.StatusChannel = gophercloudChannel

	start := time.Now()

	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	go osObjects.CreateLarge(command.Ctx.ServiceClient, containerName, objectName, stream, opts)

	statusBarsByName := map[string]*uiprogress.Bar{}
	fileNamesByBar := map[*uiprogress.Bar]string{}

	progress := uiprogress.New()
	progress.RefreshInterval = time.Second * 1

	if !params.quiet {
		progress.Start()
	}

	for status := range gophercloudChannel {
		switch status.MsgType {
		case osObjects.StatusStarted:
			statusBarInfo := statusBarsByName[status.Name]
			if statusBarInfo == nil {
				statusBar := progress.AddBar(status.TotalSize).AppendCompleted().PrependElapsed().PrependFunc(func(b *uiprogress.Bar) string {
					return fileNamesByBar[b]
				}).AppendFunc(func(b *uiprogress.Bar) string {
					return fmt.Sprintf("%s/%s", humanize.Bytes(uint64(b.Current())), humanize.Bytes(uint64(b.Total)))
				})
				statusBarsByName[status.Name] = statusBar
				fileNamesByBar[statusBar] = status.Name
			} else {
				//fileNamesByBar[statusBarInfo.bar] = status.Name
			}

		case osObjects.StatusUpdate:
			if statusBar := statusBarsByName[status.Name]; statusBar != nil {
				statusBar.Incr()
				statusBar.Set(statusBar.Current() - 1 + status.IncrementUploaded)
			}
		case osObjects.StatusSuccess:
			if statusBar := statusBarsByName[status.Name]; statusBar != nil {
				statusBar.Set(status.TotalSize)
			}
		case osObjects.StatusError:
			if statusBar := statusBarsByName[status.Name]; statusBar != nil {
				fileNamesByBar[statusBar] = fmt.Sprintf("[ERROR: %s, WILL RETRY] %s", status.Err, status.Name)
			}
		default:
			statusChannel <- status.Err
		}
	}

	close(statusChannel)

	resource.Result = fmt.Sprintf("Finished! Uploaded object [%s] to container [%s] in %s", objectName, containerName, humanize.RelTime(start, time.Now(), "", ""))
}

func (command *commandUpload) StdinField() string {
	return "file"
}

func (command *commandUpload) StreamField() string {
	return "content"
}

func (command *commandUpload) HandleStreamPipe(resource *handler.Resource) error {
	err := command.Ctx.CheckFlagsSet([]string{"name"})
	if err != nil {
		return err
	}
	resource.Params.(*paramsUpload).object = command.Ctx.CLIContext.String("name")
	resource.Params.(*paramsUpload).stream = os.Stdin
	return nil
}

func (command *commandUpload) StatusChannel(resource *handler.Resource) chan interface{} {
	return resource.Params.(*paramsUpload).statusChannel
}
