package i18n

import (
	"go/ast"
	"go/parser"
	"go/token"
	"io/fs"
	"path/filepath"
	"regexp"
	"slices"
	"strconv"
	"strings"
	"testing"
)

var verbRe = regexp.MustCompile(`%[-+# 0-9.]*[a-zA-Z%]`)

// TestCatalog checks that every literal passed to i18n.T or i18n.Error has a
// Turkish entry using the same format verbs in the same order.
func TestCatalog(t *testing.T) {
	fset := token.NewFileSet()
	seen := 0
	err := filepath.WalkDir("..", func(path string, d fs.DirEntry, err error) error {
		if err != nil || d.IsDir() || !strings.HasSuffix(path, ".go") || strings.HasSuffix(path, "_test.go") {
			return err
		}
		f, err := parser.ParseFile(fset, path, nil, 0)
		if err != nil {
			return err
		}
		ast.Inspect(f, func(n ast.Node) bool {
			call, ok := n.(*ast.CallExpr)
			if !ok || len(call.Args) != 1 {
				return true
			}
			sel, ok := call.Fun.(*ast.SelectorExpr)
			if !ok || (sel.Sel.Name != "T" && sel.Sel.Name != "Error") {
				return true
			}
			if pkg, ok := sel.X.(*ast.Ident); !ok || pkg.Name != "i18n" {
				return true
			}
			lit, ok := call.Args[0].(*ast.BasicLit)
			if !ok {
				return true
			}
			en, _ := strconv.Unquote(lit.Value)
			seen++
			tr, ok := tr[en]
			if !ok {
				t.Errorf("%s: no Turkish entry for %q", fset.Position(lit.Pos()), en)
			} else if !slices.Equal(verbRe.FindAllString(en, -1), verbRe.FindAllString(tr, -1)) {
				t.Errorf("%s: format verbs differ: %q vs %q", fset.Position(lit.Pos()), en, tr)
			}
			return true
		})
		return nil
	})
	if err != nil {
		t.Fatal(err)
	}
	if seen < 100 {
		t.Fatalf("found only %d translated literals; is the walk broken?", seen)
	}
	for en, tr := range tr {
		if !slices.Equal(verbRe.FindAllString(en, -1), verbRe.FindAllString(tr, -1)) {
			t.Errorf("format verbs differ: %q vs %q", en, tr)
		}
	}
}

func TestDetect(t *testing.T) {
	defer Set("en")
	for _, tc := range []struct {
		env  map[string]string
		want string
	}{
		{map[string]string{EnvLang: "tr"}, "tr"},
		{map[string]string{EnvLang: "en", "LANG": "tr_TR.UTF-8"}, "en"},
		{map[string]string{"LANG": "tr_TR.UTF-8"}, "tr"},
		{map[string]string{"LC_ALL": "en_US.UTF-8", "LANG": "tr_TR.UTF-8"}, "en"},
		{map[string]string{"LC_ALL": "C", "LANG": "tr_TR.UTF-8"}, "tr"},
		{map[string]string{"LANG": "en_US.UTF-8"}, "en"},
	} {
		Detect(func(k string) (string, bool) { v, ok := tc.env[k]; return v, ok })
		if Lang() != tc.want {
			t.Errorf("%v: lang = %s, want %s", tc.env, Lang(), tc.want)
		}
	}
	Set("tr")
	if T("Saved: %s\n") != "Kaydedildi: %s\n" || Error("invalid configuration").Error() != "geçersiz yapılandırma" {
		t.Error("Turkish lookup failed")
	}
	if T("not in the catalog") != "not in the catalog" {
		t.Error("missing entry should fall back to English")
	}
}
