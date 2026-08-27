package cli

import (
	"encoding/json"
	"fmt"
	"strings"

	"github.com/spf13/cobra"

	"github.com/danielino/atlas/internal/doctor"
	"github.com/danielino/atlas/internal/ledger"
)

func newDoctorCmd() *cobra.Command {
	cmd := &cobra.Command{
		Use:   "doctor",
		Short: "Check the ledger for integrity problems",
		Args:  cobra.NoArgs,
		RunE: func(cmd *cobra.Command, args []string) error {
			useJSON := jsonFlag(cmd)

			root, err := requireRoot(cmd, useJSON)
			if err != nil {
				return err
			}

			cfg, err := ledger.LoadConfig(root)
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			report, err := doctor.Run(root, cfg, doctor.Options{})
			if err != nil {
				return failIO(cmd, useJSON, err)
			}

			if useJSON {
				data, err := json.MarshalIndent(report, "", "  ")
				if err != nil {
					return failIO(cmd, true, err)
				}
				fmt.Fprintln(cmd.OutOrStdout(), string(data))
			} else {
				fmt.Fprint(cmd.OutOrStdout(), renderDoctorText(report))
			}

			if report.HasErrors() {
				return &ExitError{Code: 3}
			}
			return nil
		},
	}
	addJSONFlag(cmd)
	return cmd
}

func renderDoctorText(report doctor.Report) string {
	var b strings.Builder
	b.WriteString("# ATLAS DOCTOR\n")

	if len(report.Errors) == 0 && len(report.Warnings) == 0 {
		b.WriteString("no problems found\n")
	}

	if len(report.Errors) > 0 {
		b.WriteString("## ERRORS\n")
		for _, i := range report.Errors {
			b.WriteString("- " + i.Message + "\n")
		}
	}

	if len(report.Warnings) > 0 {
		b.WriteString("## WARNINGS\n")
		for _, i := range report.Warnings {
			b.WriteString("- " + i.Message + "\n")
		}
	}

	if len(report.Fixed) > 0 {
		b.WriteString("## FIXED\n")
		for _, f := range report.Fixed {
			b.WriteString("- " + f + "\n")
		}
	}

	return b.String()
}
