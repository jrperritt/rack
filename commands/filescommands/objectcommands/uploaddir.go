package objectcommands

import (
	"fmt"
	"os"
	"path/filepath"
	"runtime"
	"strings"
	"sync"
	"time"

	"github.com/rackspace/rack/commandoptions"
	"github.com/rackspace/rack/handler"
	"github.com/rackspace/rack/internal/github.com/cenkalti/backoff"
	"github.com/rackspace/rack/internal/github.com/codegangsta/cli"
	"github.com/rackspace/rack/internal/github.com/dustin/go-humanize"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud/openstack/objectstorage/v1/objects"
	"github.com/rackspace/rack/util"
)

var uploadDir = cli.Command{
	Name:        "upload-dir",
	Usage:       util.Usage(commandPrefix, "upload-dir", "--container <containerName> [--dir <dirName> | --stdin dir]"),
	Description: "Uploads the contents of a local directory to a container",
	Action:      actionUploadDir,
	Flags:       commandoptions.CommandFlags(flagsUploadDir, keysUploadDir),
	BashComplete: func(c *cli.Context) {
		commandoptions.CompleteFlags(commandoptions.CommandFlags(flagsUploadDir, keysUploadDir))
	},
}

func flagsUploadDir() []cli.Flag {
	return []cli.Flag{
		cli.StringFlag{
			Name:  "container",
			Usage: "[required] The name of the container to upload the objects.",
		},
		cli.StringFlag{
			Name:  "dir",
			Usage: "[optional; required if `stdin` isn't provided] The name the local directory which will be uploaded.",
		},
		cli.StringFlag{
			Name:  "stdin",
			Usage: "[optional; required if `dir` isn't provided] The field being piped to STDIN. Valid values are: dir.",
		},
		cli.StringFlag{
			Name:  "content-type",
			Usage: "[optional] The Content-Type header that will be set on all objects.",
		},
		cli.IntFlag{
			Name:  "concurrency",
			Usage: "[optional] The amount of concurrent workers that will upload the directory.",
		},
		cli.BoolFlag{
			Name:  "quiet",
			Usage: "[optional] By default, every file upload will be outputted. If --quiet is provided, only a final summary will be outputted.",
		},
		cli.BoolFlag{
			Name:  "recurse",
			Usage: "[optional] By default, only files at the root level of the specified directory are uploaded. If --recurse is provided, the upload will be fully recursive and the entire subtree uploaded.",
		},
	}
}

var keysUploadDir = []string{}

type paramsUploadDir struct {
	container   string
	dir         string
	opts        objects.CreateOpts
	concurrency int
	quiet       bool
	recurse     bool
}

type commandUploadDir handler.Command

func actionUploadDir(c *cli.Context) {
	command := &commandUploadDir{
		Ctx: &handler.Context{
			CLIContext: c,
		},
	}
	handler.Handle(command)
}

func (command *commandUploadDir) Context() *handler.Context {
	return command.Ctx
}

func (command *commandUploadDir) Keys() []string {
	return keysUploadDir
}

func (command *commandUploadDir) ServiceClientType() string {
	return serviceClientType
}

func (command *commandUploadDir) HandleFlags(resource *handler.Resource) error {
	if err := command.Ctx.CheckFlagsSet([]string{"container"}); err != nil {
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
		conc = 100
	}

	resource.Params = &paramsUploadDir{
		container:   containerName,
		dir:         c.String("dir"),
		opts:        opts,
		concurrency: conc,
		quiet:       c.Bool("quiet"),
		recurse:     c.Bool("recurse"),
	}

	return nil
}

func (command *commandUploadDir) StdinField() string {
	return "dir"
}

func (command *commandUploadDir) HandlePipe(resource *handler.Resource, item string) error {
	resource.Params.(*paramsUploadDir).dir = item
	return nil
}

func (command *commandUploadDir) HandleSingle(resource *handler.Resource) error {
	err := command.Ctx.CheckFlagsSet([]string{"dir"})
	if err != nil {
		return err
	}
	resource.Params.(*paramsUploadDir).dir = command.Ctx.CLIContext.String("dir")
	return nil
}

func (command *commandUploadDir) Execute(resource *handler.Resource) {
	params := resource.Params.(*paramsUploadDir)

	stat, err := os.Stat(params.dir)
	if err != nil {
		resource.Err = err
		return
	}
	if !stat.IsDir() {
		resource.Err = fmt.Errorf("%s is not a directory, ignoring", params.dir)
		return
	}

	// bump thread count to number of available CPUs
	runtime.GOMAXPROCS(runtime.NumCPU())

	jobs := make(chan string)
	results := make(chan *handler.Resource)

	var wg sync.WaitGroup

	totals := uploadDirSummary{RWMutex: new(sync.RWMutex)}

	start := time.Now()

	for i := 0; i < params.concurrency; i++ {
		wg.Add(1)
		go func() {
			for p := range jobs {
				var re *handler.Resource

				ticker := backoff.NewTicker(backoff.NewExponentialBackOff())
				for _ = range ticker.C {
					re = command.handle(p, params)
					if re.Err != nil {
						continue
					}

					ticker.Stop()
					break
				}

				fi, err := os.Stat(p)
				if err == nil {
					totals.Lock()
					totals.totalSize += uint64(fi.Size())
					totals.totalFiles++
					totals.Unlock()
				}

				if !params.quiet {
					command.Ctx.Results <- re
				}
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
			jobs <- path
		}
		return nil
	})
	close(jobs)

	wg.Wait()
	close(results)

	resource.Result = fmt.Sprintf("Finished! Uploaded %s %s totaling %s in %s", humanize.Comma(totals.totalFiles), util.Pluralize("object", totals.totalFiles), humanize.Bytes(totals.totalSize), humanize.RelTime(start, time.Now(), "", ""))
}

func (command *commandUploadDir) handle(p string, params *paramsUploadDir) *handler.Resource {
	re := &handler.Resource{}

	file, err := os.Open(p)
	defer file.Close()

	if err != nil {
		re.Err = err
		return re
	}

	on := strings.TrimPrefix(p, params.dir+string(os.PathSeparator))
	res := objects.Create(command.Ctx.ServiceClient, params.container, on, file, params.opts)
	re.Err = res.Err

	if res.Err == nil {
		if params.quiet == true {
			re.Result = ""
		} else {
			re.Result = fmt.Sprintf("Uploaded %s to %s", on, params.container)
		}
	}

	return re
}

type uploadDirSummary struct {
	*sync.RWMutex
	totalSize  uint64
	totalFiles int64
}
