package handler

import (
	"fmt"
	"strings"

	"github.com/rackspace/rack/auth"
	"github.com/rackspace/rack/commandoptions"
	"github.com/rackspace/rack/internal/github.com/Sirupsen/logrus"
	"github.com/rackspace/rack/internal/github.com/codegangsta/cli"
	"github.com/rackspace/rack/internal/github.com/rackspace/gophercloud"
	"github.com/rackspace/rack/output"
	"github.com/rackspace/rack/util"
)

// Command is the type that commands have.
type Command struct {
	// See `Context`
	Ctx *Context
}

// Context is a global context that `rack` uses.
type Context struct {
	// CLIContext is the context that the `cli` library uses. `rack` uses it to
	// access flags.
	CLIContext *cli.Context
	// ServiceClient is the Rackspace service client used to authenticate the user
	// and carry out the requests while processing the command.
	ServiceClient *gophercloud.ServiceClient
	// ServiceClientType is the type of Rackspace service client used (e.g. compute).
	ServiceClientType string
	// Results is a channel into which commands send results. It allows for streaming
	// output.
	Results chan *Resource

	GlobalOptions struct {
		output        string
		noCache       bool
		noHeader      bool
		useServiceNet bool
	}

	//DebugChannel chan interface{}

	// logger is used to log information acquired while processing the command.
	logger *logrus.Logger
}

func onlyNonNil(m map[string]interface{}) map[string]interface{} {
	for k, v := range m {
		if v == nil {
			m[k] = ""
		}
	}
	return m
}

// limitFields returns only the fields the user specified in the `fields` flag. If
// the flag wasn't provided, all fields are returned.
func (ctx *Context) limitFields(resource *Resource) {
	if ctx.CLIContext.IsSet("fields") {
		fields := strings.Split(strings.ToLower(ctx.CLIContext.String("fields")), ",")
		newKeys := []string{}
		for _, key := range resource.Keys {
			if util.Contains(fields, strings.Join(strings.Split(strings.ToLower(key), " "), "-")) {
				newKeys = append(newKeys, key)
			}
		}
		resource.Keys = newKeys
	}
}

// StoreCredentials caches the users auth credentials if available and the `no-cache`
// flag was not provided.
func (ctx *Context) storeCredentials() {
	// if serviceClient is nil, the HTTP request for the command didn't get sent.
	// don't set cache if the `no-cache` flag is provided
	if ctx.ServiceClient != nil && !ctx.GlobalOptions.noCache {
		newCacheValue := &auth.CacheItem{
			TokenID:         ctx.ServiceClient.TokenID,
			ServiceEndpoint: ctx.ServiceClient.Endpoint,
		}
		// get auth credentials
		credsResult, err := auth.Credentials(ctx.CLIContext, nil)
		if err != nil {
			if ctx.logger != nil {
				ctx.logger.Infof("Error storing credentials in cache: %s\n", err)
			}
			return
		}
		ao := credsResult.AuthOpts
		region := credsResult.Region
		urlType := gophercloud.AvailabilityPublic
		if ctx.GlobalOptions.useServiceNet {
			urlType = gophercloud.AvailabilityInternal
		}
		// form the cache key
		cacheKey := auth.CacheKey(*ao, region, ctx.ServiceClientType, urlType)
		// initialize the cache
		cache := &auth.Cache{}
		// set the cache value to the current values
		_ = cache.SetValue(cacheKey, newCacheValue)
	}
}

func (ctx *Context) handleGlobalOptions() error {
	defaultSection, err := commandoptions.ProfileSection("")
	if err != nil {
		return err
	}

	defaultKeysHash := map[string]string{}
	if defaultSection != nil {
		defaultKeysHash = defaultSection.KeysHash()
	}

	have := make(map[string]commandoptions.Cred)
	want := map[string]string{
		"output":          "",
		"no-cache":        "",
		"no-header":       "",
		"log":             "",
		"use-service-net": "",
	}

	// use command-line options if available
	commandoptions.CLIopts(ctx.CLIContext, have, want)
	// are there any unset auth variables?
	if len(want) != 0 {
		// if so, look in config file
		err := commandoptions.ConfigFile(ctx.CLIContext, have, want)
		if err != nil {
			return err
		}
	}

	var outputFormat string
	if ctx.CLIContext.IsSet("output") {
		outputFormat = ctx.CLIContext.String("output")
	} else if value, ok := defaultKeysHash["output"]; ok && value != "" {
		outputFormat = value
	} else {
		have["output"] = commandoptions.Cred{
			Value: "table",
			From:  "default value",
		}
		outputFormat = "table"
	}
	switch outputFormat {
	case "json", "csv", "table":
		ctx.GlobalOptions.output = outputFormat
	default:
		return fmt.Errorf("Invalid value for `output` flag: '%s'. Options are: json, csv, table.", outputFormat)
	}

	if ctx.CLIContext.IsSet("no-header") {
		ctx.GlobalOptions.noHeader = true
	} else if value, ok := defaultKeysHash["no-header"]; ok && value != "" {
		ctx.GlobalOptions.noHeader = true
	}

	if ctx.CLIContext.IsSet("no-cache") {
		ctx.GlobalOptions.noCache = true
	} else if value, ok := defaultKeysHash["no-cache"]; ok && value != "" {
		ctx.GlobalOptions.noCache = true
	}

	if ctx.CLIContext.IsSet("use-service-net") {
		ctx.GlobalOptions.useServiceNet = true
	} else if value, ok := defaultKeysHash["use-service-net"]; ok && value != "" {
		ctx.GlobalOptions.useServiceNet = true
	}

	var logLevel string
	if ctx.CLIContext.IsSet("log") {
		logLevel = ctx.CLIContext.String("log")
	} else if value, ok := defaultKeysHash["log"]; ok && value != "" {
		logLevel = value
	}
	var level logrus.Level
	if logLevel != "" {
		switch strings.ToLower(logLevel) {
		case "debug":
			level = logrus.DebugLevel
		case "info":
			level = logrus.InfoLevel
		default:
			return fmt.Errorf("Invalid value for `log` flag: %s. Valid options are: debug, info", logLevel)
		}
	}
	ctx.logger = &logrus.Logger{
		Out:       ctx.CLIContext.App.Writer,
		Formatter: &logrus.TextFormatter{},
		Level:     level,
	}

	haveString := ""
	for k, v := range have {
		haveString += fmt.Sprintf("%s: %s (from %s)\n", k, v.Value, v.From)
	}
	ctx.logger.Infof("Global Options:\n%s\n", haveString)

	return nil
}

// IDOrName is a function for retrieving a resources unique identifier based on
// whether he or she passed an `id` or a `name` flag.
func (ctx *Context) IDOrName(idFromNameFunc func(*gophercloud.ServiceClient, string) (string, error)) (string, error) {
	if ctx.CLIContext.IsSet("id") {
		if ctx.CLIContext.IsSet("name") {
			return "", fmt.Errorf("Only one of either --id or --name may be provided.")
		}
		return ctx.CLIContext.String("id"), nil
	} else if ctx.CLIContext.IsSet("name") {
		name := ctx.CLIContext.String("name")
		id, err := idFromNameFunc(ctx.ServiceClient, name)
		if err != nil {
			return "", fmt.Errorf("Error converting name [%s] to ID: %s", name, err)
		}
		return id, nil
	} else {
		return "", output.ErrMissingFlag{Msg: "One of either --id or --name must be provided."}
	}
}

// CheckArgNum checks that the provided number of arguments has the same
// cardinality as the expected number of arguments.
func (ctx *Context) CheckArgNum(expected int) error {
	argsLen := len(ctx.CLIContext.Args())
	if argsLen != expected {
		return fmt.Errorf("Expected %d args but got %d\nUsage: %s\n", expected, argsLen, ctx.CLIContext.Command.Usage)
	}
	return nil
}

// CheckFlagsSet checks that the given flag names are set for the command.
func (ctx *Context) CheckFlagsSet(flagNames []string) error {
	for _, flagName := range flagNames {
		if !ctx.CLIContext.IsSet(flagName) {
			return output.ErrMissingFlag{Msg: fmt.Sprintf("--%s is required.", flagName)}
		}
	}
	return nil
}

// CheckKVFlag is a function used for verifying the format of a key-value flag.
func (ctx *Context) CheckKVFlag(flagName string) (map[string]string, error) {
	kv := make(map[string]string)
	kvStrings := strings.Split(ctx.CLIContext.String(flagName), ",")
	for _, kvString := range kvStrings {
		temp := strings.Split(kvString, "=")
		if len(temp) != 2 {
			return nil, output.ErrFlagFormatting{Msg: fmt.Sprintf("Expected key1=value1,key2=value2 format but got %s for --%s.\n", kvString, flagName)}
		}
		kv[temp[0]] = temp[1]
	}
	return kv, nil
}

// CheckStructFlag is a function used for verifying the format of a struct flag.
func (ctx *Context) CheckStructFlag(flagValues []string) ([]map[string]interface{}, error) {
	valSliceMap := make([]map[string]interface{}, len(flagValues))
	for i, flagValue := range flagValues {
		kvStrings := strings.Split(flagValue, ",")
		m := make(map[string]interface{})
		for _, kvString := range kvStrings {
			temp := strings.Split(kvString, "=")
			if len(temp) != 2 {
				return nil, output.ErrFlagFormatting{Msg: fmt.Sprintf("Expected key1=value1,key2=value2 format but got %s.\n", kvString)}
			}
			m[temp[0]] = temp[1]
		}
		valSliceMap[i] = m
	}
	return valSliceMap, nil
}
