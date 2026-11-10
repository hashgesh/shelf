package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hashgesh/shelf/internal/i18n"
)

// usageTemplate is cobra's default usage template; i18n.T translates it.
const usageTemplate = `Usage:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [command]{{end}}{{if gt (len .Aliases) 0}}

Aliases:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Examples:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Commands:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Flags:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Global flags:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Use "{{.CommandPath}} [command] --help" for more information about a command.{{end}}
`

// localize applies the translated usage template to cmd and its children.
// Cobra appends an untranslated "[flags]" to use lines, so that is disabled
// and commands spell out "[flags]" themselves.
func localize(cmd *cobra.Command) {
	cmd.SetUsageTemplate(i18n.T(usageTemplate))
	cmd.DisableFlagsInUseLine = true
	for _, c := range cmd.Commands() {
		localize(c)
	}
}

// exactArgs is cobra.ExactArgs with a translated message.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return fmt.Errorf(i18n.T("expected %d arguments, got %d (usage: %s)"), n, len(args), cmd.UseLine())
		}
		return nil
	}
}

// maxArgs is cobra.MaximumNArgs with a translated message.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return fmt.Errorf(i18n.T("at most %d arguments allowed, got %d (usage: %s)"), n, len(args), cmd.UseLine())
		}
		return nil
	}
}

// noArgs is cobra.NoArgs with a translated message.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf(i18n.T("unexpected argument %q (see: %s --help)"), args[0], cmd.CommandPath())
	}
	return nil
}

func newHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   i18n.T("help [command]"),
		Short: i18n.T("Show help for a command"),
		RunE: func(c *cobra.Command, args []string) error {
			target, _, err := c.Root().Find(args)
			if target == nil || err != nil {
				return c.Root().Help()
			}
			return target.Help()
		},
	}
}
