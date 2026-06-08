package cli

import (
	"fmt"
	"os"
	"strings"

	"github.com/spf13/cobra"
	"gopkg.in/yaml.v3"

	"github.com/flowctl/flowctl/internal/huml"
	"github.com/flowctl/flowctl/internal/model"
	"github.com/flowctl/flowctl/internal/validator"
)

var workflowCmd = &cobra.Command{
	Use:     "workflow",
	Aliases: []string{"wf"},
	Short:   "Manage workflows",
}

func init() {
	// workflow create
	createCmd := &cobra.Command{
		Use:   "create",
		Short: "Create a workflow from a YAML or HUML file",
		RunE:  runWorkflowCreate,
	}
	createCmd.Flags().StringP("file", "f", "", "Path to workflow definition file (YAML or HUML)")
	_ = createCmd.MarkFlagRequired("file")

	// workflow list
	listCmd := &cobra.Command{
		Use:   "list",
		Short: "List all workflows",
		RunE:  runWorkflowList,
	}

	// workflow get
	getCmd := &cobra.Command{
		Use:   "get <slug>",
		Short: "Get workflow details",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkflowGet,
	}

	// workflow run
	runCmd := &cobra.Command{
		Use:   "run <slug>",
		Short: "Start a workflow execution",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkflowRun,
	}
	runCmd.Flags().StringArray("input", nil, "Input values in key=value format (can be repeated)")

	// workflow validate
	validateCmd := &cobra.Command{
		Use:   "validate",
		Short: "Validate a workflow definition file offline",
		RunE:  runWorkflowValidate,
	}
	validateCmd.Flags().StringP("file", "f", "", "Path to workflow definition file (YAML or HUML)")
	_ = validateCmd.MarkFlagRequired("file")

	// workflow rollback
	rollbackCmd := &cobra.Command{
		Use:   "rollback <slug>",
		Short: "Rollback a workflow to a specific version",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkflowRollback,
	}
	rollbackCmd.Flags().Int("to-version", 0, "Version number to rollback to")
	_ = rollbackCmd.MarkFlagRequired("to-version")

	// workflow versions
	versionsCmd := &cobra.Command{
		Use:   "versions <slug>",
		Short: "List all versions of a workflow",
		Args:  cobra.ExactArgs(1),
		RunE:  runWorkflowVersions,
	}

	workflowCmd.AddCommand(createCmd, listCmd, getCmd, runCmd, validateCmd, rollbackCmd, versionsCmd)
}

func runWorkflowCreate(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	// Determine format and parse to validate structure before sending
	var sourceFormat string
	if strings.HasSuffix(filePath, ".huml") {
		sourceFormat = "huml"
	} else {
		sourceFormat = "yaml"
	}

	reqBody := map[string]interface{}{
		"source_format": sourceFormat,
		"source_raw":    string(data),
	}

	client := NewAPIClient()
	var result map[string]interface{}
	if err := client.Post("/api/v1/workflows", reqBody, &result); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(result)
	}

	fmt.Printf("Workflow created successfully.\n")
	if slug, ok := result["slug"]; ok {
		fmt.Printf("Slug: %v\n", slug)
	}
	if id, ok := result["id"]; ok {
		fmt.Printf("ID:   %v\n", id)
	}
	return nil
}

func runWorkflowList(cmd *cobra.Command, args []string) error {
	client := NewAPIClient()
	var workflows []map[string]interface{}
	if err := client.Get("/api/v1/workflows", &workflows); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(workflows)
	}

	headers := []string{"SLUG", "NAME", "DESCRIPTION", "UPDATED"}
	rows := make([][]string, 0, len(workflows))
	for _, wf := range workflows {
		rows = append(rows, []string{
			fmt.Sprintf("%v", wf["slug"]),
			fmt.Sprintf("%v", wf["name"]),
			fmt.Sprintf("%v", wf["description"]),
			fmt.Sprintf("%v", wf["updated_at"]),
		})
	}
	out.PrintTable(headers, rows)
	return nil
}

func runWorkflowGet(cmd *cobra.Command, args []string) error {
	slug := args[0]
	client := NewAPIClient()
	var workflow map[string]interface{}
	if err := client.Get(fmt.Sprintf("/api/v1/workflows/%s", slug), &workflow); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(workflow)
	}

	fmt.Printf("Slug:        %v\n", workflow["slug"])
	fmt.Printf("Name:        %v\n", workflow["name"])
	fmt.Printf("Description: %v\n", workflow["description"])
	fmt.Printf("ID:          %v\n", workflow["id"])
	fmt.Printf("Created:     %v\n", workflow["created_at"])
	fmt.Printf("Updated:     %v\n", workflow["updated_at"])
	return nil
}

func runWorkflowRun(cmd *cobra.Command, args []string) error {
	slug := args[0]
	inputFlags, _ := cmd.Flags().GetStringArray("input")

	inputs := make(map[string]interface{})
	for _, kv := range inputFlags {
		parts := strings.SplitN(kv, "=", 2)
		if len(parts) != 2 {
			return fmt.Errorf("invalid input format %q, expected key=value", kv)
		}
		inputs[parts[0]] = parts[1]
	}

	reqBody := map[string]interface{}{
		"inputs": inputs,
	}

	client := NewAPIClient()
	var result map[string]interface{}
	if err := client.Post(fmt.Sprintf("/api/v1/workflows/%s/execute", slug), reqBody, &result); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(result)
	}

	fmt.Printf("Execution started.\n")
	if id, ok := result["id"]; ok {
		fmt.Printf("Execution ID: %v\n", id)
	}
	if status, ok := result["status"]; ok {
		fmt.Printf("Status:       %v\n", status)
	}
	return nil
}

func runWorkflowValidate(cmd *cobra.Command, args []string) error {
	filePath, _ := cmd.Flags().GetString("file")

	data, err := os.ReadFile(filePath)
	if err != nil {
		return fmt.Errorf("reading file: %w", err)
	}

	var def *model.WorkflowDefinition

	if strings.HasSuffix(filePath, ".huml") {
		def, err = huml.ParseHUML(string(data))
		if err != nil {
			return fmt.Errorf("parsing HUML: %w", err)
		}
	} else {
		def = &model.WorkflowDefinition{}
		if err := yaml.Unmarshal(data, def); err != nil {
			return fmt.Errorf("parsing YAML: %w", err)
		}
	}

	result := validator.ValidateWorkflow(def)

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(result)
	}

	if result.Valid {
		fmt.Println("Workflow is valid.")
		return nil
	}

	fmt.Println("Validation failed:")
	for _, e := range result.Errors {
		fmt.Printf("  - %s: %s\n", e.Field, e.Message)
	}
	return fmt.Errorf("validation failed with %d error(s)", len(result.Errors))
}

func runWorkflowRollback(cmd *cobra.Command, args []string) error {
	slug := args[0]
	version, _ := cmd.Flags().GetInt("to-version")

	reqBody := map[string]interface{}{
		"version": version,
	}

	client := NewAPIClient()
	var result map[string]interface{}
	if err := client.Post(fmt.Sprintf("/api/v1/workflows/%s/rollback", slug), reqBody, &result); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(result)
	}

	fmt.Printf("Workflow %q rolled back to version %d.\n", slug, version)
	return nil
}

func runWorkflowVersions(cmd *cobra.Command, args []string) error {
	slug := args[0]
	client := NewAPIClient()
	var versions []map[string]interface{}
	if err := client.Get(fmt.Sprintf("/api/v1/workflows/%s/versions", slug), &versions); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(versions)
	}

	headers := []string{"VERSION", "SCHEMA", "FORMAT", "CHECKSUM", "PUBLISHED", "CREATED"}
	rows := make([][]string, 0, len(versions))
	for _, v := range versions {
		published := ""
		if v["published_at"] != nil {
			published = fmt.Sprintf("%v", v["published_at"])
		}
		rows = append(rows, []string{
			fmt.Sprintf("%v", v["version"]),
			fmt.Sprintf("%v", v["schema_version"]),
			fmt.Sprintf("%v", v["source_format"]),
			fmt.Sprintf("%v", v["checksum"]),
			published,
			fmt.Sprintf("%v", v["created_at"]),
		})
	}
	out.PrintTable(headers, rows)
	return nil
}
