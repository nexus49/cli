package bindNamespace

import "github.com/kyma-project/cli/internal/cli"

type Options struct {
	*cli.Options
	Name                string
	Namespace           string
	CreateIfNotExisting bool
}

//NewOptions creates options with default values
func NewOptions(o *cli.Options) *Options {
	return &Options{Options: o}
}
