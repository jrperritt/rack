package servercommands

import (
	"fmt"
	"os"

	"github.com/codegangsta/cli"
	"github.com/jrperritt/rackcli/auth"
	"github.com/jrperritt/rackcli/util"
	osServers "github.com/rackspace/gophercloud/openstack/compute/v2/servers"
	"github.com/rackspace/gophercloud/rackspace/compute/v2/servers"
)

var resize = cli.Command{
	Name:        "resize",
	Usage:       fmt.Sprintf("%s %s resize %s [--flavorID <flavorID>] [optional flags]", util.Name, commandPrefix, idOrNameUsage),
	Description: "Rebuilds an existing server",
	Action:      commandResize,
	Flags:       util.CommandFlags(flagsResize),
	BashComplete: func(c *cli.Context) {
		util.CompleteFlags(util.CommandFlags(flagsResize))
	},
}

func flagsResize() []cli.Flag {
	cf := []cli.Flag{
		cli.StringFlag{
			Name:  "flavorID",
			Usage: "[required] The ID of the flavor that the resized server should have.",
		},
	}
	return append(cf, idAndNameFlags...)
}

func commandResize(c *cli.Context) {
	util.CheckArgNum(c, 0)
	if !c.IsSet("flavorID") {
		util.PrintError(c, util.ErrMissingFlag{
			Msg: "--flavorID is required.",
		})
	}
	client := auth.NewClient("compute")
	serverID := idOrName(c, client)
	opts := osServers.ResizeOpts{
		FlavorRef: c.String("flavorID"),
	}
	err := servers.Resize(client, serverID, opts).ExtractErr()
	if err != nil {
		fmt.Printf("Error resizing server (%s): %s\n", serverID, err)
		os.Exit(1)
	}
}