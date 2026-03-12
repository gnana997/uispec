package main

import (
	"fmt"

	"github.com/spf13/cobra"
)

func newWatchCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "watch",
		Short: "Watch for file changes (coming soon)",
		Long:  "Watches the component library for changes and automatically re-scans.",
		Annotations: map[string]string{"group": "server"},
		RunE: func(cmd *cobra.Command, args []string) error {
			fmt.Println("uispec watch — not yet implemented")
			return nil
		},
	}
}
