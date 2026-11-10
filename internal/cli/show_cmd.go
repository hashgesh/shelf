package cli

import (
	"fmt"
	"io"
	"strings"

	"github.com/spf13/cobra"

	"github.com/hashgesh/shelf/internal/conan"
)

type showJSON struct {
	Ref            string          `json:"ref"`
	RecipeRevision string          `json:"recipe_revision,omitempty"` // empty with --all-revisions
	Filter         string          `json:"filter,omitempty"`
	Binaries       []*conan.Binary `json:"binaries"`
}

// filterFlags registers the shared binary filter flags on cmd.
func filterFlags(cmd *cobra.Command, f *conan.Filter) {
	fs := cmd.Flags()
	fs.StringVar(&f.OS, "os", "", "işletim sistemi (önek, ör. win, linux)")
	fs.StringVar(&f.Arch, "arch", "", "mimari (önek, ör. x86_64, armv8)")
	fs.StringVar(&f.Compiler, "compiler", "", "derleyici ve sürüm (önek, ör. gcc, gcc12, msvc194)")
	fs.StringVar(&f.BuildType, "build", "", "build type (önek, ör. Release, Debug)")
	fs.StringVar(&f.Profile, "profile", "", "eşleşen profil adı (içerir)")
}

func (a *app) newShowCmd() *cobra.Command {
	var (
		filter       conan.Filter
		allRevisions bool
	)
	cmd := &cobra.Command{
		Use:   "show <ad>[/<sürüm>[@user/channel]] [bayraklar]",
		Short: "Bir paketin binary tablosunu göster",
		Long: "Bir paket sürümünün binary'lerini gösterir. Sürüm verilmezse en yeni sürüm,\n" +
			"--all-revisions verilmezse en yeni recipe revision kullanılır.",
		Example: "  shelf show zlib\n" +
			"  shelf show zlib/1.3.1 --os win --build release\n" +
			"  shelf show fmt/10.2.1@mycompany/stable --json",
		Args: exactArgs(1),
		RunE: func(cmd *cobra.Command, args []string) error {
			spec, err := conan.ParseRefSpec(args[0])
			if err != nil {
				return err
			}
			s, err := a.load(cmd)
			if err != nil {
				return err
			}
			p, v, err := s.result.Catalog.Resolve(spec)
			if err != nil {
				return noResultError{msg: err.Error()}
			}

			var binaries []*conan.Binary
			rrev := ""
			if allRevisions {
				for _, r := range v.Revisions {
					binaries = append(binaries, r.Binaries...)
				}
			} else if r := v.Latest(); r != nil {
				rrev = r.Revision
				binaries = r.Binaries
			}
			binaries = filter.Apply(binaries)

			out := cmd.OutOrStdout()
			if a.flags.json {
				if binaries == nil {
					binaries = []*conan.Binary{}
				}
				if err := writeJSON(out, showJSON{Ref: v.Ref(p.Name), RecipeRevision: rrev, Filter: filter.Describe(), Binaries: binaries}); err != nil {
					return err
				}
				if len(binaries) == 0 {
					return noResultError{}
				}
				return nil
			}

			st := a.newStyler(out)
			header := st.bold(v.Ref(p.Name))
			switch {
			case allRevisions:
				header += fmt.Sprintf("  (%d recipe revision)", len(v.Revisions))
			case len(v.Revisions) > 1:
				header += fmt.Sprintf("  rrev %s (en yenisi; toplam %d, tümü için --all-revisions)", shortID(rrev), len(v.Revisions))
			default:
				header += "  rrev " + shortID(rrev)
			}
			fmt.Fprintln(out, header)
			if others := otherVersions(p, v); others != "" {
				fmt.Fprintln(out, st.dim("Diğer sürümler: "+others))
			}
			if !filter.IsZero() {
				fmt.Fprintln(out, st.dim("Filtre: "+filter.Describe()))
			}
			fmt.Fprintln(out)

			if len(binaries) == 0 {
				return noResultError{msg: "Eşleşen binary yok."}
			}
			renderBinaries(out, st, binaries, allRevisions)
			fmt.Fprintf(out, "\n%d binary%s%s\n", len(binaries), st.sep(), s.repoLabel)
			return nil
		},
	}
	filterFlags(cmd, &filter)
	cmd.Flags().BoolVar(&allRevisions, "all-revisions", false, "tüm recipe revision'ları göster")
	return cmd
}

func otherVersions(p *conan.Package, cur *conan.Version) string {
	var out []string
	for _, v := range p.Versions {
		if v != cur {
			out = append(out, strings.TrimPrefix(v.Ref(p.Name), p.Name+"/"))
		}
	}
	return strings.Join(out, ", ")
}

func renderBinaries(w io.Writer, st styler, bs []*conan.Binary, withRrev bool) {
	headers := []string{"OS", "ARCH", "COMPILER", "BUILD", "SHARED", "BOYUT", "TARİH", "YÜKLEYEN", "ID", "PROFİL"}
	if withRrev {
		headers = append(headers, "RREV")
	}
	t := &table{headers: headers}
	unknown := map[int]bool{}
	for i, b := range bs {
		compiler := strings.TrimSpace(b.Compiler + " " + b.CompilerVersion)
		row := []string{
			orDash(b.OS), orDash(b.Arch), orDash(compiler), orDash(b.BuildType), orDash(b.Shared()),
			humanSize(b.Size), formatTime(b.Created), orDash(b.CreatedBy), shortID(b.PackageID), b.Label(),
		}
		if withRrev {
			row = append(row, shortID(b.RecipeRevision))
		}
		t.add(row...)
		unknown[i] = b.Profile == ""
	}
	t.style = func(row, col int, s string) string {
		if col == 9 && unknown[row] {
			return st.dim(s)
		}
		return s
	}
	t.render(w, st)
}
