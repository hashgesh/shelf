package cli

import (
	"fmt"

	"github.com/spf13/cobra"
)

// usageTemplate is cobra's default usage template translated to Turkish.
const usageTemplate = `Kullanım:{{if .Runnable}}
  {{.UseLine}}{{end}}{{if .HasAvailableSubCommands}}
  {{.CommandPath}} [komut]{{end}}{{if gt (len .Aliases) 0}}

Diğer adlar:
  {{.NameAndAliases}}{{end}}{{if .HasExample}}

Örnekler:
{{.Example}}{{end}}{{if .HasAvailableSubCommands}}

Komutlar:{{range .Commands}}{{if (or .IsAvailableCommand (eq .Name "help"))}}
  {{rpad .Name .NamePadding }} {{.Short}}{{end}}{{end}}{{end}}{{if .HasAvailableLocalFlags}}

Bayraklar:
{{.LocalFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableInheritedFlags}}

Genel bayraklar:
{{.InheritedFlags.FlagUsages | trimTrailingWhitespaces}}{{end}}{{if .HasAvailableSubCommands}}

Bir komut hakkında ayrıntı için "{{.CommandPath}} [komut] --help" kullanın.{{end}}
`

// localize applies the Turkish usage template to cmd and its children. Cobra
// appends an English "[flags]" to use lines, so that is disabled and commands
// spell out "[bayraklar]" themselves.
func localize(cmd *cobra.Command) {
	cmd.SetUsageTemplate(usageTemplate)
	cmd.DisableFlagsInUseLine = true
	for _, c := range cmd.Commands() {
		localize(c)
	}
}

// exactArgs is cobra.ExactArgs with a Turkish message.
func exactArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) != n {
			return fmt.Errorf("%d argüman bekleniyordu, %d verildi (kullanım: %s)", n, len(args), cmd.UseLine())
		}
		return nil
	}
}

// maxArgs is cobra.MaximumNArgs with a Turkish message.
func maxArgs(n int) cobra.PositionalArgs {
	return func(cmd *cobra.Command, args []string) error {
		if len(args) > n {
			return fmt.Errorf("en fazla %d argüman verilebilir, %d verildi (kullanım: %s)", n, len(args), cmd.UseLine())
		}
		return nil
	}
}

// noArgs is cobra.NoArgs with a Turkish message.
func noArgs(cmd *cobra.Command, args []string) error {
	if len(args) > 0 {
		return fmt.Errorf("beklenmeyen argüman %q (yardım için: %s --help)", args[0], cmd.CommandPath())
	}
	return nil
}

func newHelpCmd() *cobra.Command {
	return &cobra.Command{
		Use:   "help [komut]",
		Short: "Bir komut hakkında yardım göster",
		RunE: func(c *cobra.Command, args []string) error {
			target, _, err := c.Root().Find(args)
			if target == nil || err != nil {
				return c.Root().Help()
			}
			return target.Help()
		},
	}
}
