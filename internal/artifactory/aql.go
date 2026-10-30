package artifactory

import (
	"encoding/json"
	"fmt"
	"strings"
	"time"
)

// DefaultInclude is the field list requested for package items. Non-admin
// users must always include repo, path and name.
var DefaultInclude = []string{
	"repo", "path", "name", "size", "created", "created_by", "modified", "modified_by", "property.*",
}

// ItemsQuery describes an items.find(...) AQL query.
type ItemsQuery struct {
	Repos   []string // OR-ed; at least one is required
	Names   []string // OR-ed file names, e.g. "conan_package.tgz"; empty means any
	Include []string // defaults to DefaultInclude
	Offset  int
	Limit   int // 0 means no limit clause
}

// String renders the query in AQL syntax. Values are JSON-encoded so repo
// and file names cannot break out of the criteria object.
func (q ItemsQuery) String() string {
	criteria := map[string]any{}
	if len(q.Repos) == 1 {
		criteria["repo"] = q.Repos[0]
	} else if len(q.Repos) > 1 {
		criteria["$or"] = eqList("repo", q.Repos)
	}
	if len(q.Names) == 1 {
		criteria["name"] = q.Names[0]
	} else if len(q.Names) > 1 {
		// A second "$or" key would collide with the repo one, so nest both in "$and".
		names := map[string]any{"$or": eqList("name", q.Names)}
		if or, ok := criteria["$or"]; ok {
			delete(criteria, "$or")
			criteria["$and"] = []any{map[string]any{"$or": or}, names}
		} else {
			criteria["$or"] = names["$or"]
		}
	}
	// json.Marshal sorts map keys, so the output is deterministic.
	crit, _ := json.Marshal(criteria)

	include := q.Include
	if len(include) == 0 {
		include = DefaultInclude
	}
	inc, _ := json.Marshal(include)

	var b strings.Builder
	fmt.Fprintf(&b, "items.find(%s).include(%s)", crit, strings.Trim(string(inc), "[]"))
	if q.Limit > 0 {
		// A stable order is required for offset paging to be meaningful.
		b.WriteString(`.sort({"$asc":["repo","path","name"]})`)
		if q.Offset > 0 {
			fmt.Fprintf(&b, ".offset(%d)", q.Offset)
		}
		fmt.Fprintf(&b, ".limit(%d)", q.Limit)
	}
	return b.String()
}

func eqList(field string, values []string) []map[string]string {
	out := make([]map[string]string, 0, len(values))
	for _, v := range values {
		out = append(out, map[string]string{field: v})
	}
	return out
}

// Result is the decoded AQL response.
type Result struct {
	Results []Item `json:"results"`
	Range   Range  `json:"range"`
}

// Range is the paging metadata Artifactory returns with every AQL response.
type Range struct {
	StartPos int `json:"start_pos"`
	EndPos   int `json:"end_pos"`
	Total    int `json:"total"`
	Limit    int `json:"limit,omitempty"`
}

// Item is one artifact row.
type Item struct {
	Repo       string     `json:"repo"`
	Path       string     `json:"path"`
	Name       string     `json:"name"`
	Type       string     `json:"type,omitempty"`
	Size       int64      `json:"size"`
	Created    time.Time  `json:"created"`
	CreatedBy  string     `json:"created_by"`
	Modified   time.Time  `json:"modified"`
	ModifiedBy string     `json:"modified_by"`
	Properties []Property `json:"properties,omitempty"`
}

// Property is a key/value pair. A key may appear several times.
type Property struct {
	Key   string `json:"key"`
	Value string `json:"value"`
}

// Key identifies an item uniquely within Artifactory.
func (i Item) Key() string { return i.Repo + "/" + i.Path + "/" + i.Name }

// FullPath is the item path inside its repository, including the file name.
func (i Item) FullPath() string {
	if i.Path == "" || i.Path == "." {
		return i.Name
	}
	return i.Path + "/" + i.Name
}

// Props groups properties by key, keeping every value in response order.
func (i Item) Props() map[string][]string {
	m := make(map[string][]string, len(i.Properties))
	for _, p := range i.Properties {
		m[p.Key] = append(m[p.Key], p.Value)
	}
	return m
}

// Prop returns the first value of key and whether it was present.
func (i Item) Prop(key string) (string, bool) {
	for _, p := range i.Properties {
		if p.Key == key {
			return p.Value, true
		}
	}
	return "", false
}
