package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

var approvalCmd = &cobra.Command{
	Use:   "approval",
	Short: "Manage workflow approvals",
}

func init() {
	// approval list
	listApprovalCmd := &cobra.Command{
		Use:   "list",
		Short: "List pending approvals",
		RunE:  runApprovalList,
	}

	// approval approve
	approveCmd := &cobra.Command{
		Use:   "approve <id>",
		Short: "Approve a pending approval",
		Args:  cobra.ExactArgs(1),
		RunE:  runApprovalApprove,
	}
	approveCmd.Flags().String("comment", "", "Approval comment")

	// approval reject
	rejectCmd := &cobra.Command{
		Use:   "reject <id>",
		Short: "Reject a pending approval",
		Args:  cobra.ExactArgs(1),
		RunE:  runApprovalReject,
	}
	rejectCmd.Flags().String("reason", "", "Rejection reason")

	approvalCmd.AddCommand(listApprovalCmd, approveCmd, rejectCmd)
}

func runApprovalList(cmd *cobra.Command, args []string) error {
	client := NewAPIClient()
	var approvals []map[string]interface{}
	if err := client.Get("/api/v1/approvals?status=pending", &approvals); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(approvals)
	}

	headers := []string{"ID", "EXECUTION", "STEP", "ROLES", "REQUESTED"}
	rows := make([][]string, 0, len(approvals))
	for _, a := range approvals {
		roles := ""
		if r, ok := a["required_roles"]; ok {
			roles = fmt.Sprintf("%v", r)
		}
		rows = append(rows, []string{
			fmt.Sprintf("%v", a["id"]),
			fmt.Sprintf("%v", a["execution_id"]),
			fmt.Sprintf("%v", a["step_id"]),
			roles,
			fmt.Sprintf("%v", a["requested_at"]),
		})
	}
	out.PrintTable(headers, rows)
	return nil
}

func runApprovalApprove(cmd *cobra.Command, args []string) error {
	id := args[0]
	comment, _ := cmd.Flags().GetString("comment")

	reqBody := map[string]interface{}{
		"status":  "approved",
		"comment": comment,
	}

	client := NewAPIClient()
	var result map[string]interface{}
	if err := client.Post(fmt.Sprintf("/api/v1/approvals/%s/respond", id), reqBody, &result); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(result)
	}

	fmt.Printf("Approval %s approved.\n", id)
	return nil
}

func runApprovalReject(cmd *cobra.Command, args []string) error {
	id := args[0]
	reason, _ := cmd.Flags().GetString("reason")

	reqBody := map[string]interface{}{
		"status":  "rejected",
		"comment": reason,
	}

	client := NewAPIClient()
	var result map[string]interface{}
	if err := client.Post(fmt.Sprintf("/api/v1/approvals/%s/respond", id), reqBody, &result); err != nil {
		return err
	}

	out := NewOutputFormatter()
	if out.Format == "json" {
		return out.PrintJSON(result)
	}

	fmt.Printf("Approval %s rejected.\n", id)
	return nil
}
