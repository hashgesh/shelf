package cli

import "github.com/spf13/cobra"

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
