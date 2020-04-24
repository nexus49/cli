package debug

import "github.com/kyma-project/cli/internal/cli"

type Options struct {
	*cli.Options
	WorkingDir string
}

//NewOptions creates options with default values
func NewOptions(o *cli.Options) *Options {
	return &Options{Options: o}
}
