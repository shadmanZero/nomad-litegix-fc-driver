package main

import (
	"github.com/hashicorp/go-hclog"
	"github.com/hashicorp/nomad/plugins"
	"github.com/shadm/nomad-litegix-fc-driver/litegix"
)

func main() {
	// Serve the plugin
	plugins.Serve(func(log hclog.Logger) interface{} {
		return factory(log)
	})
}

func factory(log hclog.Logger) interface{} {
	return litegix.NewPlugin(log)
}

