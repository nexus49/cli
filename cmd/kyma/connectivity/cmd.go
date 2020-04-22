package connectivity

import (
	"github.com/spf13/cobra"
)

//NewCmd creates a new provision command
func NewCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "connectivity",
		Short: "Manages connected applications",
	}
	return cmd
}
