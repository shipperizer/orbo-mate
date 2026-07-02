package cmd

import (
	"fmt"
	"github.com/shipperizer/orbo-mate/pkg/version"
	"github.com/spf13/cobra"
)

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version number of orbo-mate",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Printf("orbo-mate version %s\n", version.Version)
	},
}

func init() {
	rootCmd.AddCommand(versionCmd)
}
