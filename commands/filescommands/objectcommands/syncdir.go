package objectcommands

import (
	"fmt"
	"math"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rackspace/rack/commandoptions"
	"github.com/rackspace/rack/handler"
	"github.com/rackspace/rack/internal/github.com/codegangsta/cli"
	"github.com/rackspace/rack/internal/github.com/dustin/go-humanize"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/openstack/objectstorage/v1/containers"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/rackspace/rack/util"
)

var syncFromDir = cli.Command{
	Name:  "sync-from-dir",
	Usage: util.Usage(commandPrefix, "sync-dir", "--container <containerName>"),
	Description: "Alters the contents of a container to match a local directory, " +
		"leaving the local directory unchanged.",
	Action: actionSyncFromDir,
	Flags:  commandoptions.CommandFlags(flagsSyncFromDir, keysSyncFromDir),
	BashComplete: func(c *cli.Context) {
		commandoptions.CompleteFlags(commandoptions.CommandFlags(flagsSyncFromDir, keysSyncFromDir))
	},
}

func flagsSyncFromDir() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "container",
			Usage: "[required] The name of the container to sync the objects.",
		},
		cli.StringFlag{
			Name:  "dir",
			Usage: "[required] The name the local directory which will be synced.",
		},
		cli.StringFlag{
			Name:  "content-type",
			Usage: "[optional] The Content-Type header that will be set on all objects.",
		},
		cli.IntFlag{
			Name:  "concurrency",
			Usage: "[optional] The amount of concurrent workers that will sync the directory.",
		},
		cli.BoolFlag{
			Name: "quiet",
			Usage: "[optional] By default, every file sync will be outputted. If " +
				"--quiet is provided, only a final summary will be outputted.",
		},
		cli.BoolFlag{
			Name: "recurse",
			Usage: "[optional] By default, only files at the root level of the " +
				"specified directory are synced. If --recurse is provided, the sync " +
				"will be fully recursive and the entire subtree synced.",
		},
		cli.BoolFlag{
			Name: "prune",
			Usage: "[optional] If provided, objects already in the container that " +
				"are not in `dir` will be deleted.",
		},
		cli.StringFlag{
			Name: "comparison-method",
			Usage: "[optional] The method to use for comparing whether 2 files are " +
				"the same. Options: md5, size-and-date. Default is size-and-date.",
		},
		cli.StringFlag{
			Name: "path-separator",
			Usage: "[optional] The character used to separate paths. Default is" +
				"your operating system's default ('/' for Unix, '\\' for Windows).",
		},
	}
}

var keysSyncFromDir = []string{}

type paramsSyncFromDir struct {
	container        string
	dir              string
	opts             objects.CreateOpts
	concurrency      int
	quiet            bool
	recurse          bool
	prune            bool
	comparisonMethod string
	pathSeparator    rune
}

type commandSyncFromDir handler.Command

func actionSyncFromDir(c *cli.Context) {
	command := &commandSyncFromDir{
		Ctx: &handler.Context{
			CLIContext: c,
		},
	}
	handler.Handle(command)
}

func (command *commandSyncFromDir) Context() *handler.Context {
	return command.Ctx
}

func (command *commandSyncFromDir) Keys() []string {
	return keysSyncFromDir
}

func (command *commandSyncFromDir) ServiceClientType() string {
	return serviceClientType
}

func (command *commandSyncFromDir) HandleFlags(resource *handler.Resource) error {
	if err := command.Ctx.CheckFlagsSet([]string{"container", "dir"}); err != nil {
		return err
	}

	c := command.Ctx.CLIContext
	containerName := c.String("container")
	if err := CheckContainerExists(command.Ctx.ServiceClient, containerName); err != nil {
		return err
	}

	opts := objects.CreateOpts{
		ContentType: c.String("content-type"),
	}

	conc := c.Int("concurrency")
	if conc <= 0 {
		conc = 1
	}

	comparisonMethod := c.String("comparison-method")
	if comparisonMethod == "" || comparisonMethod == "size-and-date" {
		comparisonMethod = ""
	} else if c.String("comparison-method") != "md5" {
		return fmt.Errorf("Invalid value (%s) for flag (%s)", comparisonMethod, "comparison-method")
	}

	pathSeparator := filepath.Separator
	if ps := c.String("path-separator"); ps != "" {
		pathSeparator = rune(ps[0])
	}

	resource.Params = &paramsSyncFromDir{
		container:        containerName,
		dir:              c.String("dir"),
		opts:             opts,
		concurrency:      conc,
		quiet:            c.Bool("quiet"),
		recurse:          c.Bool("recurse"),
		prune:            c.Bool("prune"),
		comparisonMethod: comparisonMethod,
		pathSeparator:    pathSeparator,
	}

	return nil
}

func (command *commandSyncFromDir) Execute(resource *handler.Resource) {
	params := resource.Params.(*paramsSyncFromDir)

	//command.dirMinusContainer(params.dir, params.container)

	//command.uploadFiles()

	stat, err := os.Stat(params.dir)
	if err != nil {
		resource.Err = err
		return
	}
	if !stat.IsDir() {
		resource.Err = fmt.Errorf("%s is not a directory, ignoring", params.dir)
		return
	}

	// if GOMAXPROCS not set, bump thread count to number of available CPUs
	if os.Getenv("GOMAXPROCS") == "" {
		runtime.GOMAXPROCS(runtime.NumCPU())
	}

	results := make(chan *handler.Resource)

	var wg sync.WaitGroup

	totals := syncFromDirSummary{RWMutex: new(sync.RWMutex)}

	start := time.Now()

	// if we need to delete objects from the container that don't exist locally
	if params.prune {
		// get the container info
		container, err := containers.Get(command.Ctx.ServiceClient, params.container).Extract()
		// if there was a problem getting the container info
		if err != nil {
			// container doesn't exist; exit
		}

		// the number ob objects from the container each goroutine should check.
		// we convert the number of objects and number of goroutines to floating
		// point, divide them, and then round up to overestimate the number of
		// objects in the container.
		numObjectsPerGoroutine := int(math.Ceil(float64(container.ObjectCount) / float64(params.concurrency)))

		// for each goroutine we spawn to prune objects from the container
		for i := 1; i <= params.concurrency; i++ {
			// set the variable that will be local to each goroutine
			i := i
			// add 1 to the waitgroup
			wg.Add(1)
			// spawn the goroutine
			go func() {
				//
				allPages, err := objects.List(command.Ctx.ServiceClient, params.container, objects.ListOpts{
					Limit: i * numObjectsPerGoroutine,
				}).AllPages()
				if err != nil {
					// error listing this page of objects
				}

				objectNames, err := objects.ExtractNames(allPages)
				if err != nil {
					// error extracting names
				}

				parent := filepath.Clean(params.dir)
				// for each object name in the slice of object names
				for _, objectName := range objectNames {
					// join the parent path with the relative path from the container
					path := filepath.Join(parent, objectName)
					// if the there's an error stat-ing the file path
					if _, err := os.Stat(path); err != nil {
						// if the error is for the file not existing locally
						if os.IsNotExist(err) {
							// delete it from container.
							res := objects.Delete(command.Ctx.ServiceClient, params.container, objectName, nil)
							// if we encountered an error deleting the object from the container
							if res.Err != nil {
								// send the error back to the user
							}
						}
						// else the error is for something other than the file not existing
					}
					// else file exists; do nothing
				}

				/*
					var re *handler.Resource

					if !params.quiet {
						command.Ctx.Results <- re
					}
				*/

				// this goroutine has finished, so subtract 1 from the waitgroup.
				wg.Done()
			}()
		}
		// wait until all the goroutines have finished pruning objects from
		// the container before continuing.
		wg.Wait()
	}

	// create a channel into which we can put the local file [info] that we
	// need to check in the container.
	filesToCheckChannel := make(chan os.FileInfo)

	//
	for i := 0; i < params.concurrency; i++ {
		wg.Add(1)
		go func() {
			for fi := range filesToCheckChannel {

				objectRaw := objects.Get(command.Ctx.ServiceClient, params.container, fi.Name(), nil)

				objectHeaders, err := objectRaw.Extract()
				if err != nil {

				}

				/*
					var re *handler.Resource

					re = command.checkDir(fi, params)
					if re.Err != nil {
						continue
					}

					if !params.quiet {
						command.Ctx.Results <- re
					}
				*/
			}
			wg.Done()
		}()
	}

	filepath.Walk(params.dir, func(path string, info os.FileInfo, err error) error {
		pathSep := string(os.PathSeparator)
		parent := filepath.Clean(params.dir)
		if !params.recurse && strings.Contains(strings.TrimPrefix(path, parent+pathSep), pathSep) {
			return nil
		}
		if !info.IsDir() {
			filesToCheckChannel <- info
		}
		return nil
	})

	close(filesToCheckChannel)
	wg.Wait()

	close(results)

	resource.Result = fmt.Sprintf("Finished! Synced %s %s totaling %s in %s",
		humanize.Comma(totals.totalFiles),
		util.Pluralize("object", totals.totalFiles),
		humanize.Bytes(totals.totalSize),
		humanize.RelTime(start, time.Now(), "", ""))
}

func (command *commandSyncFromDir) prune(params *paramsSyncFromDir) {

}

func (command *commandSyncFromDir) checkDirs(fi os.FileInfo, params *paramsSyncFromDir) []string {
	re := &handler.Resource{}

	file, err := os.Open(fi.Name())
	defer file.Close()

	if err != nil {
		re.Err = err
		return []string{}
	}

	on := strings.TrimPrefix(fi.Name(), params.dir+string(os.PathSeparator))
	res := objects.Create(command.Ctx.ServiceClient, params.container, on, file, params.opts)
	re.Err = res.Err

	if res.Err == nil {
		if params.quiet == true {
			re.Result = ""
		} else {
			re.Result = fmt.Sprintf("Synced %s to %s", on, params.container)
		}
	}

	return []string{}
}

type syncFromDirSummary struct {
	*sync.RWMutex
	totalSize  uint64
	totalFiles int64
}

func (command *commandSyncFromDir) dirMinusContainer(dir, container string) []string {
	return []string{}
}
