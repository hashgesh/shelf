package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hashgesh/shelf/internal/conan"
	"github.com/hashgesh/shelf/internal/i18n"
)

type matrixCellJSON struct {
	State      conan.CellState `json:"state"`
	PackageIDs []string        `json:"package_ids"`
}

type matrixRowJSON struct {
	Package string                    `json:"package"`
	Ref     string                    `json:"ref"`
	Cells   map[string]matrixCellJSON `json:"cells"`
}

type matrixJSON struct {
	Profiles []string        `json:"profiles"`
	Rows     []matrixRowJSON `json:"rows"`
	Missing  int             `json:"missing"`
}

func (a *app) newMatrixCmd() *cobra.Command {
	var (
		filter      conan.Filter
		allVersions bool
	)
	cmd := &cobra.Command{
		Use:   i18n.T("matrix [package-filter] [flags]"),
		Short: i18n.T("Package × profile table: which profiles have a binary"),
		Long: i18n.T("For the newest version of each package, shows whether a binary matches each\n") +
			i18n.T("profile in the profiles folder. Filter flags select the columns (profiles).\n") +
			i18n.T("✓ available, ✗ missing, ? unclear because some binaries have unknown settings."),
		Example: "  shelf matrix\n  shelf matrix --build Release\n  shelf matrix zlib --os win --ascii",
		Args:    maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.load(cmd)
			if err != nil {
				return err
			}
			if len(s.result.Profiles) == 0 {
				return noResultError{msg: fmt.Sprintf(i18n.T("No profiles found: %s (--profiles-dir or the profiles_dir setting)"), s.cfg.ProfilesDir)}
			}
			var pkgs []*conan.Package
			for _, p := range s.result.Catalog.Packages {
				if len(args) == 0 || containsFold(p.Name, args[0]) {
					pkgs = append(pkgs, p)
				}
			}
			if pkgs == nil {
				pkgs = []*conan.Package{}
			}
			m := conan.BuildMatrix(s.result.Catalog, s.result.Profiles, conan.MatrixOptions{
				Filter: filter, AllVersions: allVersions, Packages: pkgs,
			})

			out := cmd.OutOrStdout()
			if a.flags.json {
				v := matrixJSON{Profiles: []string{}, Rows: []matrixRowJSON{}, Missing: m.Missing()}
				for _, p := range m.Profiles {
					v.Profiles = append(v.Profiles, p.Name)
				}
				for _, r := range m.Rows {
					row := matrixRowJSON{Package: r.Package, Ref: r.Ref, Cells: map[string]matrixCellJSON{}}
					for i, c := range r.Cells {
						ids := []string{}
						for _, b := range c.Binaries {
							ids = append(ids, b.PackageID)
						}
						row.Cells[m.Profiles[i].Name] = matrixCellJSON{State: c.State, PackageIDs: ids}
					}
					v.Rows = append(v.Rows, row)
				}
				if err := writeJSON(out, v); err != nil {
					return err
				}
			}
			if len(m.Rows) == 0 || len(m.Profiles) == 0 {
				return noResultError{msg: i18n.T("No matching packages or profiles.")}
			}
			if a.flags.json {
				return nil
			}

			st := a.newStyler(out)
			t := &table{headers: []string{i18n.T("PACKAGE")}}
			for _, p := range m.Profiles {
				t.headers = append(t.headers, p.Name)
			}
			for _, r := range m.Rows {
				cells := []string{r.Ref}
				for _, c := range r.Cells {
					cells = append(cells, st.cellSymbol(c.State))
				}
				t.add(cells...)
			}
			t.style = func(row, col int, s string) string {
				if col == 0 {
					return st.bold(s)
				}
				return st.colorCell(m.Rows[row].Cells[col-1].State, s)
			}
			t.render(out, st)
			unit := i18n.T("packages")
			if allVersions {
				unit = i18n.T("versions")
			}
			fmt.Fprintf(out, i18n.T("\n%d %s%s%d profiles%s%d missing%s%s\n"), len(m.Rows), unit, st.times(), len(m.Profiles), st.sep(), m.Missing(), st.sep(), s.repoLabel)
			return nil
		},
	}
	filterFlags(cmd, &filter)
	cmd.Flags().BoolVar(&allVersions, "all-versions", false, i18n.T("show a separate row for each version"))
	return cmd
}
