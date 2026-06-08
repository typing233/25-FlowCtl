package cli

import (
	"fmt"
	"os"
	"path/filepath"

	"github.com/spf13/cobra"
	"github.com/spf13/viper"
)

var tenantCmd = &cobra.Command{
	Use:   "tenant",
	Short: "Manage tenants",
}

func init() {
	// tenant list
	listTenantCmd := &cobra.Command{
		Use:   "list",
		Short: "List tenants you belong to",
		RunE:  runTenantList,
	}

	// tenant switch
	switchCmd := &cobra.Command{
		Use:   "switch <slug>",
		Short: "Switch active tenant",
		Args:  cobra.ExactArgs(1),
		RunE:  runTenantSwitch,
	}

	tenantCmd.AddCommand(listTenantCmd, switchCmd)
}

func runTenantList(cmd *cobra.Command, args []string) error {
	client := NewAPIClient()
	var tenants []map[string]interface{}
	if err := client.Get("/api/v1/tenants", &tenants); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(tenants)
	}

	headers := []string{"SLUG", "NAME", "ACTIVE"}
	rows := make([][]string, 0, len(tenants))
	for _, t := range tenants {
		active := ""
		slug := fmt.Sprintf("%v", t["slug"])
		if slug == cfgTenant {
			active = "*"
		}
		rows = append(rows, []string{
			slug,
			fmt.Sprintf("%v", t["name"]),
			active,
		})
	}
	out.PrintTable(headers, rows)
	return nil
}

func runTenantSwitch(cmd *cobra.Command, args []string) error {
	slug := args[0]

	home, err := os.UserHomeDir()
	if err != nil {
		return fmt.Errorf("getting home directory: %w", err)
	}

	configDir := filepath.Join(home, ".flowctl")
	if err := os.MkdirAll(configDir, 0700); err != nil {
		return fmt.Errorf("creating config directory: %w", err)
	}

	configPath := filepath.Join(configDir, "config.yaml")
	viper.Set("tenant", slug)
	viper.SetConfigFile(configPath)
	if err := viper.WriteConfig(); err != nil {
		if err := viper.SafeWriteConfig(); err != nil {
			return fmt.Errorf("writing config: %w", err)
		}
	}

	cfgTenant = slug
	fmt.Printf("Switched to tenant %q.\n", slug)
	return nil
}
