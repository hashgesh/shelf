package cli

import (
	"fmt"

	"github.com/spf13/cobra"

	"github.com/hashgesh/shelf/internal/conan"
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
		Use:   "matrix [paket-filtresi] [bayraklar]",
		Short: "Paket × profil tablosu: hangi profil için binary var",
		Long: "Her paketin en yeni sürümü için, profil klasöründeki her profile uyan bir\n" +
			"binary olup olmadığını gösterir. Filtre bayrakları sütunları (profilleri) seçer.\n" +
			"✓ var, ✗ yok, ? ayarları bilinmeyen binary'ler yüzünden belirsiz.",
		Example: "  shelf matrix\n  shelf matrix --build Release\n  shelf matrix zlib --os win --ascii",
		Args:    maxArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			s, err := a.load(cmd)
			if err != nil {
				return err
			}
			if len(s.result.Profiles) == 0 {
				return noResultError{msg: fmt.Sprintf("Profil bulunamadı: %s (--profiles-dir veya profiles_dir ayarı)", s.cfg.ProfilesDir)}
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
				return noResultError{msg: "Eşleşen paket veya profil yok."}
			}
			if a.flags.json {
				return nil
			}

			st := a.newStyler(out)
			t := &table{headers: []string{"PAKET"}}
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
			unit := "paket"
			if allVersions {
				unit = "sürüm"
			}
			fmt.Fprintf(out, "\n%d %s%s%d profil%s%d eksik%s%s\n", len(m.Rows), unit, st.times(), len(m.Profiles), st.sep(), m.Missing(), st.sep(), s.repoLabel)
			return nil
		},
	}
	filterFlags(cmd, &filter)
	cmd.Flags().BoolVar(&allVersions, "all-versions", false, "her sürüm için ayrı satır göster")
	return cmd
}
