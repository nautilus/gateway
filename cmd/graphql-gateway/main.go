package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"
)

var rootCmd = &cobra.Command{
	Use:   "graphql-gateway",
	Short: "GraphQL Gateway is a standalone service to consolidate your GraphQL APIs.",
}

// start the gateway executable
func main() {
	if err := rootCmd.Execute(); err != nil {
		fmt.Println(err)
		os.Exit(1)
	}
}
