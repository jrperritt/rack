package objectcommands

import (
	"github.com/rackspace/rack/commandoptions"
	"github.com/rackspace/rack/handler"
	"github.com/rackspace/rack/internal/github.com/codegangsta/cli"
	"github.com/rackspace/rack/internal/github.com/fatih/structs"
	osObjects "github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/pagination"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/rackspace/objectstorage/v1/objects"
	"github.com/rackspace/rack/util"
)

var list = cli.Command{
	Name:        "list",
	Usage:       util.Usage(commandPrefix, "list", "[--container <containerName> | --stdin container]"),
	Description: "Lists objects in a container",
	Action:      actionList,
	Flags:       commandoptions.CommandFlags(flagsList, keysList),
	BashComplete: func(c *cli.Context) {
		commandoptions.CompleteFlags(commandoptions.CommandFlags(flagsList, keysList))
	},
}

func flagsList() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "container",
			Usage: "[optional; required if `stdin` isn't provided] The name of the container",
		},
		cli.StringFlag{
			Name:  "stdin",
			Usage: "[optional; required if `container` isn't provided] The field being piped into STDIN. Valid values are: container",
		},
		cli.BoolFlag{
			Name:  "all-pages",
			Usage: "[optional] Return all objects. Default is to paginate.",
		},
		cli.StringFlag{
			Name:  "prefix",
			Usage: "[optional] Only return objects with this prefix.",
		},
		cli.StringFlag{
			Name:  "end-marker",
			Usage: "[optional] Only return objects with name less than this value.",
		},
		cli.StringFlag{
			Name:  "marker",
			Usage: "[optional] Start listing objects at this object name.",
		},
		cli.IntFlag{
			Name:  "limit",
			Usage: "[optional] Only return this many objects at most.",
		},
		cli.StringFlag{
			Name:  "delimiter",
			Usage: "[optional] Return objects names that are nested in the container.",
		},
	}
}

var keysList = []string{"Name", "Bytes", "ContentType", "LastModified"}

type paramsList struct {
	container string
	opts      *osObjects.ListOpts
	allPages  bool
}

type commandList handler.Command

func actionList(c *cli.Context) {
	command := &commandList{
		Ctx: &handler.Context{
			CLIContext: c,
		},
	}
	handler.Handle(command)
}

func (command *commandList) Context() *handler.Context {
	return command.Ctx
}

func (command *commandList) Keys() []string {
	return keysList
}

func (command *commandList) ServiceClientType() string {
	return serviceClientType
}

func (command *commandList) HandleFlags(resource *handler.Resource) error {
	err := command.Ctx.CheckFlagsSet([]string{"container"})
	if err != nil {
		return err
	}

	c := command.Ctx.CLIContext

	delimiter := ""
	if d := c.String("delimiter"); d != "" {
		delimiter = d
	}

	opts := &osObjects.ListOpts{
		Full:      c.Bool("full"),
		Prefix:    c.String("prefix"),
		EndMarker: c.String("end-marker"),
		Marker:    c.String("marker"),
		Limit:     c.Int("limit"),
		Delimiter: delimiter,
	}

	resource.Params = &paramsList{
		opts:     opts,
		allPages: c.Bool("all-pages"),
	}
	return nil
}

func (command *commandList) HandlePipe(resource *handler.Resource, item string) error {
	resource.Params.(*paramsList).container = item
	return nil
}

func (command *commandList) HandleSingle(resource *handler.Resource) error {
	err := command.Ctx.CheckFlagsSet([]string{"container"})
	if err != nil {
		return err
	}
	resource.Params.(*paramsList).container = command.Ctx.CLIContext.String("container")
	return nil
}

func (command *commandList) Execute(resource *handler.Resource) {
	opts := resource.Params.(*paramsList).opts
	opts.Full = true
	containerName := resource.Params.(*paramsList).container
	allPages := resource.Params.(*paramsList).allPages
	pager := objects.List(command.Ctx.ServiceClient, containerName, opts)
	if allPages {
		pages, err := pager.AllPages()
		if err != nil {
			resource.Err = err
			return
		}
		info, err := objects.ExtractInfo(pages)
		if err != nil {
			resource.Err = err
			return
		}
		result := make([]map[string]interface{}, len(info))
		for j, obj := range info {
			result[j] = structs.Map(&obj)
		}
		resource.Result = result
	} else {
		limit := opts.Limit
		err := pager.EachPage(func(page pagination.Page) (bool, error) {
			info, err := objects.ExtractInfo(page)
			if err != nil {
				return false, err
			}
			result := make([]map[string]interface{}, len(info))
			for j, obj := range info {
				result[j] = structs.Map(&obj)
			}
			resource.Result = result
			if len(info) >= limit {
				return false, nil
			}
			limit -= len(info)
			command.Ctx.Results <- resource
			return true, nil
		})
		if err != nil {
			resource.Err = err
			return
		}
	}
}

func (command *commandList) StdinField() string {
	return "container"
}
