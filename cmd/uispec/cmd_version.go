package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newVersionCmd(ver, cmt, dt string) *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version information",
		Annotations: map[string]string{"group": "info"},
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("uispec %s %s\n",
				styleBold(ver),
				styleDim(fmt.Sprintf("(commit: %s, built: %s)", cmt, dt)))
		},
	}
}
