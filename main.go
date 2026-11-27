package main

//go:generate go tool tfplugindocs generate --provider-name polaris

import (
	"github.com/ErikBPF/terraform-provider-polaris/polaris"
	"github.com/hashicorp/terraform-plugin-sdk/v2/plugin"
)

var version = "dev"

func main() {
	plugin.Serve(&plugin.ServeOpts{ProviderFunc: polaris.New(version), ProviderAddr: "registry.terraform.io/ErikBPF/polaris"})
}
