// Stella 是 OneBot 聊天机器人。
package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vksir/stella/internal/app"
)

func main() {
	var cfgPath string

	rootCmd := &cobra.Command{
		Use:   "stella",
		Short: "Stella 聊天机器人",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Run(cfgPath)
		},
	}
	rootCmd.Flags().StringVarP(&cfgPath, "config", "c", "stella.toml", "配置文件路径")

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}
