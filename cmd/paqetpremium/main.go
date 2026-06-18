package main

import (
	"fmt"
	"os"
	"time"

	"github.com/paqetpremium/paqetpremium/internal/app"
	"github.com/paqetpremium/paqetpremium/internal/version"
	"github.com/spf13/cobra"
)

func main() {
	if err := newRootCmd().Execute(); err != nil {
		os.Exit(1)
	}
}

func newRootCmd() *cobra.Command {
	root := &cobra.Command{
		Use:   "paqetpremium",
		Short: "PaqetPremium packet-level tunnel core",
	}

	root.AddCommand(
		newRunCmd(),
		newVersionCmd(),
		newTestCmd(),
		newReloadCmd(),
		newBenchCmd(),
	)

	return root
}

func newRunCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "run",
		Short: "Run tunnel from config file",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				return fmt.Errorf("config path is required (-c)")
			}
			return app.RunConfig(configPath)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to YAML config")
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

func newVersionCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "version",
		Short: "Print version",
		Run: func(cmd *cobra.Command, args []string) {
			fmt.Printf("%s %s\n", version.Name, version.Version)
		},
	}
}

func newTestCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "test",
		Short: "Validate config and run connectivity checks",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				return fmt.Errorf("config path is required (-c)")
			}

			res, err := app.TestConfig(configPath)
			if err != nil {
				return err
			}

			for _, line := range res.Checks {
				fmt.Println(line)
			}

			if !res.OK {
				return fmt.Errorf("one or more checks failed")
			}

			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to YAML config")
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

func newReloadCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "reload",
		Short: "Hot-reload running client via admin API",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				return fmt.Errorf("config path is required (-c)")
			}
			return app.ReloadConfig(configPath)
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to YAML config")
	_ = cmd.MarkFlagRequired("config")

	return cmd
}

func newBenchCmd() *cobra.Command {
	var configPath string

	cmd := &cobra.Command{
		Use:   "bench",
		Short: "Measure KCP ping latency to upstream servers",
		RunE: func(cmd *cobra.Command, args []string) error {
			if configPath == "" {
				return fmt.Errorf("config path is required (-c)")
			}

			lines, err := app.BenchConfig(configPath)
			if err != nil {
				return err
			}

			for _, line := range lines {
				if line.OK {
					fmt.Printf("%s %s  %s\n", line.Name, line.Addr, line.RTT.Round(time.Millisecond))
				} else {
					fmt.Printf("%s %s  FAIL: %s\n", line.Name, line.Addr, line.Err)
				}
			}
			return nil
		},
	}

	cmd.Flags().StringVarP(&configPath, "config", "c", "", "path to YAML config")
	_ = cmd.MarkFlagRequired("config")

	return cmd
}
