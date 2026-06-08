package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var (
	cfgServer string
	cfgToken  string
	cfgTenant string
	cfgOutput string
)

var rootCmd = &cobra.Command{
	Use:   "flowctl",
	Short: "FlowCtl - Workflow orchestration CLI",
	Long:  "FlowCtl is a command-line tool for managing workflows, executions, and approvals.",
	PersistentPreRunE: func(cmd *cobra.Command, args []string) error {
		return loadConfig()
	},
	SilenceUsage:  true,
	SilenceErrors: true,
}

func init() {
	rootCmd.PersistentFlags().StringVar(&cfgServer, "server", "http://localhost:8080", "API server URL")
	rootCmd.PersistentFlags().StringVar(&cfgToken, "token", "", "Authentication token")
	rootCmd.PersistentFlags().StringVar(&cfgTenant, "tenant", "", "Tenant slug")
	rootCmd.PersistentFlags().StringVar(&cfgOutput, "output", "table", "Output format (json or table)")

	_ = viper.BindPFlag("server", rootCmd.PersistentFlags().Lookup("server"))
	_ = viper.BindPFlag("token", rootCmd.PersistentFlags().Lookup("token"))
	_ = viper.BindPFlag("tenant", rootCmd.PersistentFlags().Lookup("tenant"))
	_ = viper.BindPFlag("output", rootCmd.PersistentFlags().Lookup("output"))

	rootCmd.AddCommand(loginCmd)
	rootCmd.AddCommand(workflowCmd)
	rootCmd.AddCommand(executionCmd)
	rootCmd.AddCommand(approvalCmd)
	rootCmd.AddCommand(tenantCmd)
	rootCmd.AddCommand(versionCmd)
}

func loadConfig() error {
	home, err := os.UserHomeDir()
	if err != nil {
		return nil // non-fatal; defaults are fine
	}

	configDir := filepath.Join(home, ".flowctl")
	viper.SetConfigName("config")
	viper.SetConfigType("yaml")
	viper.AddConfigPath(configDir)

	viper.SetEnvPrefix("FLOWCTL")
	viper.AutomaticEnv()

	if err := viper.ReadInConfig(); err != nil {
		// config file not found is OK, we use flags/defaults
		if _, ok := err.(viper.ConfigFileNotFoundError); !ok {
			return nil
		}
	}

	// Propagate viper values back to variables if flags were not explicitly set
	if cfgServer == "http://localhost:8080" && viper.GetString("server") != "" {
		cfgServer = viper.GetString("server")
	}
	if cfgToken == "" && viper.GetString("token") != "" {
		cfgToken = viper.GetString("token")
	}
	if cfgTenant == "" && viper.GetString("tenant") != "" {
		cfgTenant = viper.GetString("tenant")
	}
	if cfgOutput == "table" && viper.GetString("output") != "" {
		cfgOutput = viper.GetString("output")
	}

	return nil
}

var versionCmd = &cobra.Command{
	Use:   "version",
	Short: "Print the version of flowctl",
	Run: func(cmd *cobra.Command, args []string) {
		fmt.Println("flowctl version 0.1.0")
	},
}

// Execute runs the root command.
func Execute() error {
	return rootCmd.Execute()
}
