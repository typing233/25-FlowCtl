package cli

import (
	"encoding/json"
	"fmt"
	"os"
	"strings"
	"text/tabwriter"
)

// OutputFormatter handles rendering data in table or JSON format.
type OutputFormatter struct {
	Format string
}

// NewOutputFormatter creates a formatter based on the global output flag.
func NewOutputFormatter() *OutputFormatter {
	return &OutputFormatter{Format: cfgOutput}
}

// PrintJSON pretty-prints the given value as JSON.
func (f *OutputFormatter) PrintJSON(v interface{}) error {
	data, err := json.MarshalIndent(v, "", "  ")
	if err != nil {
		return fmt.Errorf("marshaling JSON: %w", err)
	}
	fmt.Println(string(data))
	return nil
}

// PrintTable prints data as a text table with headers.
func (f *OutputFormatter) PrintTable(headers []string, rows [][]string) {
	w := tabwriter.NewWriter(os.Stdout, 0, 0, 2, ' ', 0)
	// Print header
	fmt.Fprintln(w, strings.Join(headers, "\t"))
	// Print separator
	seps := make([]string, len(headers))
	for i, h := range headers {
		seps[i] = strings.Repeat("-", len(h))
	}
	fmt.Fprintln(w, strings.Join(seps, "\t"))
	// Print rows
	for _, row := range rows {
		fmt.Fprintln(w, strings.Join(row, "\t"))
	}
	w.Flush()
}

// Print prints data either as JSON or table based on the configured format.
// For JSON output, jsonData is used. For table output, headers and rows are used.
func (f *OutputFormatter) Print(jsonData interface{}, headers []string, rows [][]string) error {
	if f.Format == "json" {
		return f.PrintJSON(jsonData)
	}
	f.PrintTable(headers, rows)
	return nil
}
