package cli

import (
	"fmt"
	"strconv"
	"strings"
	"time"

	"github.com/spf13/cobra"

	"github.com/hashgesh/shelf/internal/i18n"
)

type listVersionJSON struct {
	Ref          string    `json:"ref"`
	Version      string    `json:"version"`
	User         string    `json:"user,omitempty"`
	Channel      string    `json:"channel,omitempty"`
	Revisions    int       `json:"revisions"`
	Binaries     int       `json:"binaries"` // in the latest recipe revision
	LatestUpload time.Time `json:"latest_upload"`
}

type listPackageJSON struct {
	Name     string            `json:"name"`
	Versions []listVersionJSON `json:"versions"`
}

func (a *app) newListCmd() *cobra.Command {
	return &cobra.Command{
		Use:   i18n.T("list [filter]"),
		Short: i18n.T("List packages and their versions"),
		Long: i18n.T("Lists packages and their versions. With a filter, only packages whose name\n") +
			i18n.T("contains the text are shown. The BINARY column counts binaries in the newest recipe revision."),
		Example: "  shelf list\n  shelf list zlib\n  shelf list --json",
		Args:    maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.load(cmd)
			if err != nil {
				return err
			}

			var pkgs []listPackageJSON
			for _, p := range s.result.Catalog.Packages {
				if len(args) == 1 && !containsFold(p.Name, args[0]) {
					continue
				}
				lp := listPackageJSON{Name: p.Name}
				for _, v := range p.Versions {
					lv := listVersionJSON{
						Ref: v.Ref(p.Name), Version: v.Version, User: v.User, Channel: v.Channel,
						Revisions: len(v.Revisions),
					}
					if r := v.Latest(); r != nil {
						lv.Binaries = len(r.Binaries)
					}
					for _, r := range v.Revisions {
						for _, b := range r.Binaries {
							if b.Created.After(lv.LatestUpload) {
								lv.LatestUpload = b.Created
							}
						}
					}
					lp.Versions = append(lp.Versions, lv)
				}
				pkgs = append(pkgs, lp)
			}

			if a.flags.json {
				if pkgs == nil {
					pkgs = []listPackageJSON{}
				}
				if err := writeJSON(cmd.OutOrStdout(), pkgs); err != nil {
					return err
				}
			} else if len(pkgs) > 0 {
				st := a.newStyler(cmd.OutOrStdout())
				t := &table{headers: []string{i18n.T("PACKAGE"), i18n.T("VERSION"), i18n.T("REVISION"), "BINARY", i18n.T("LAST UPLOAD")}}
				for _, p := range pkgs {
					for i, v := range p.Versions {
						name := p.Name
						if i > 0 {
							name = ""
						}
						ver := v.Version
						if v.User != "" || v.Channel != "" {
							ver = strings.TrimPrefix(v.Ref, p.Name+"/")
						}
						t.add(name, ver, strconv.Itoa(v.Revisions), strconv.Itoa(v.Binaries), formatTime(v.LatestUpload))
					}
				}
				t.style = func(_, col int, s string) string {
					if col == 0 {
						return st.bold(s)
					}
					return s
				}
				t.render(cmd.OutOrStdout(), st)
				fmt.Fprintf(cmd.OutOrStdout(), i18n.T("\n%d packages%s%s\n"), len(pkgs), st.sep(), s.repoLabel)
			}
			if len(pkgs) == 0 {
				return noResultError{msg: i18n.T("No packages found.")}
			}
			return nil
		},
	}
}

func containsFold(s, sub string) bool {
	return strings.Contains(strings.ToLower(s), strings.ToLower(sub))
}
