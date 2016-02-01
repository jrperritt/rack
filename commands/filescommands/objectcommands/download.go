package objectcommands

import (
	"io"
	"io/ioutil"
	"time"

	"github.com/rackspace/rack/commandoptions"
	"github.com/rackspace/rack/handler"
	"github.com/rackspace/rack/internal/github.com/codegangsta/cli"
	osObjects "github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/rackspace/objectstorage/v1/objects"
	"github.com/rackspace/rack/util"
)

var download = cli.Command{
	Name:        "download",
	Usage:       util.Usage(commandPrefix, "download", "--container <containerName> --name <objectName>"),
	Description: "Downloads an object",
	Action:      actionDownload,
	Flags:       commandoptions.CommandFlags(flagsDownload, keysDownload),
	BashComplete: func(c *cli.Context) {
		commandoptions.CompleteFlags(commandoptions.CommandFlags(flagsDownload, keysDownload))
	},
}

func flagsDownload() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "container",
			Usage: "[required] The name of the container containing the object to download",
		},
		cli.StringFlag{
			Name:  "name",
			Usage: "[required] The name of the object to download",
		},
	}
}

var keysDownload = []string{}

type paramsDownload struct {
	container string
	object    string
}

type commandDownload handler.Command

func actionDownload(c *cli.Context) {
	command := &commandDownload{
		Ctx: &handler.Context{
			CLIContext: c,
		},
	}
	handler.Handle(command)
}

func (command *commandDownload) Context() *handler.Context {
	return command.Ctx
}

func (command *commandDownload) Keys() []string {
	return keysDownload
}

func (command *commandDownload) ServiceClientType() string {
	return serviceClientType
}

func (command *commandDownload) HandleFlags(resource *handler.Resource) error {
	err := command.Ctx.CheckFlagsSet([]string{"container", "name"})
	if err != nil {
		return err
	}

	c := command.Ctx.CLIContext
	containerName := c.String("container")
	if err := CheckContainerExists(command.Ctx.ServiceClient, containerName); err != nil {
		return err
	}

	object := c.String("name")
	resource.Params = &paramsDownload{
		container: containerName,
		object:    object,
	}
	return nil
}

func (command *commandDownload) Execute(resource *handler.Resource) {
	containerName := resource.Params.(*paramsDownload).container
	objectName := resource.Params.(*paramsDownload).object
	rawResponse := objects.Download(command.Ctx.ServiceClient, containerName, objectName, nil)
	if rawResponse.Err != nil {
		resource.Err = rawResponse.Err
		return
	}

	resource.Result = contentWrapperTransferProgress{
		userContent: rawResponse.Body,
	}
}

func (command *commandDownload) JSON(resource *handler.Resource) {
	bytes, err := ioutil.ReadAll(resource.Result.(io.Reader))
	if err != nil {
		resource.Err = err
		return
	}
	resource.Result = string(bytes)
}

// contentWrapperTransferProgress is a wrapper to track the progress of an
// HTTP transfer such as a file upload or download.
type contentWrapperTransferProgress struct {
	userContent     io.ReadCloser
	name            string
	totalSize       int
	increment       int
	startTime       time.Time
	responseChannel chan *osObjects.TransferStatus
}

func (cwtp contentWrapperTransferProgress) Read(p []byte) (int, error) {
	n, err := cwtp.userContent.Read(p)
	cwtp.responseChannel <- &osObjects.TransferStatus{
		Name:      cwtp.name,
		TotalSize: cwtp.totalSize,
		Increment: n,
		MsgType:   osObjects.StatusUpdate,
		StartTime: cwtp.startTime,
	}
	return n, err
}
