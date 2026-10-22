package main

import (
	"fmt"
	"os"

	"github.com/spf13/cobra"

	"github.com/vksir/stella/internal/app"
	"github.com/vksir/stella/internal/infra/daemon"
)

func main() {
	var cfgPath string

	rootCmd := &cobra.Command{
		Use:   "stella",
		Short: "Stella Agent",
		RunE: func(cmd *cobra.Command, args []string) error {
			return app.Run(cfgPath)
		},
	}
	rootCmd.Flags().StringVarP(&cfgPath, "config", "c", "stella.toml", "config path")
	rootCmd.AddCommand(daemonCmd(&cfgPath))

	if err := rootCmd.Execute(); err != nil {
		fmt.Fprintln(os.Stderr, err)
		os.Exit(1)
	}
}

func daemonCmd(cfgPath *string) *cobra.Command {
	var noAutostart bool

	cmd := &cobra.Command{
		Use:   "daemon",
		Short: "Manage the stella daemon service",
	}
	cmd.PersistentFlags().StringVarP(cfgPath, "config", "c", "stella.toml", "config path")

	install := &cobra.Command{
		Use:   "install",
		Short: "Register the daemon as an OS service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Install(*cfgPath, !noAutostart)
		},
	}
	install.Flags().BoolVar(&noAutostart, "no-autostart", false, "do not start the daemon at boot")

	start := &cobra.Command{
		Use:   "start",
		Short: "Start the daemon (install it first if absent)",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Start(*cfgPath, !noAutostart)
		},
	}
	start.Flags().BoolVar(&noAutostart, "no-autostart", false, "do not start the daemon at boot")

	stop := &cobra.Command{
		Use:   "stop",
		Short: "Stop the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Stop()
		},
	}

	restart := &cobra.Command{
		Use:   "restart",
		Short: "Restart the daemon",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Restart()
		},
	}

	status := &cobra.Command{
		Use:   "status",
		Short: "Show daemon status",
		RunE: func(cmd *cobra.Command, args []string) error {
			st, err := daemon.QueryStatus()
			if err != nil {
				return err
			}
			printStatus(st)
			return nil
		},
	}

	uninstall := &cobra.Command{
		Use:   "uninstall",
		Short: "Stop and remove the daemon service",
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Uninstall()
		},
	}

	run := &cobra.Command{
		Use:    "run",
		Short:  "Run as a service (invoked by the OS service manager)",
		Hidden: true,
		RunE: func(cmd *cobra.Command, args []string) error {
			return daemon.Run(*cfgPath)
		},
	}

	cmd.AddCommand(install, start, stop, restart, status, uninstall, run)
	return cmd
}

func printStatus(st daemon.Status) {
	fmt.Printf("service:   %s\n", daemon.Name)
	fmt.Printf("state:     %s\n", st.State)
	if st.PID > 0 {
		fmt.Printf("pid:       %d\n", st.PID)
	}
	if st.Installed {
		fmt.Printf("autostart: %s\n", onOff(st.Autostart))
	}
	if st.ConfigPath != "" {
		fmt.Printf("config:    %s\n", st.ConfigPath)
	}
	if st.LogPath != "" {
		fmt.Printf("log:       %s\n", st.LogPath)
	}
}

func onOff(b bool) string {
	if b {
		return "on"
	}
	return "off"
}
