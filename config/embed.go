package presets

import (
	"embed"
	"path"
	"strings"
)

//go:embed *.toml
var files embed.FS

const Default = "default"

const ext = ".toml"

func Names() []string { return append([]string(nil), names...) }

func ByName(name string) (string, bool) {
	if name == "" {
		name = Default
	}
	body, ok := bodies[name]
	return body, ok
}

var names, bodies = func() ([]string, map[string]string) {
	bodies := map[string]string{}
	var rest []string

	entries, err := files.ReadDir(".")
	if err != nil {
		return nil, bodies
	}
	for _, e := range entries {
		if e.IsDir() || path.Ext(e.Name()) != ext {
			continue
		}
		body, err := files.ReadFile(e.Name())
		if err != nil {
			continue
		}
		name := strings.TrimSuffix(e.Name(), ext)
		bodies[name] = string(body)
		if name != Default {
			rest = append(rest, name)
		}
	}

	var names []string
	if _, ok := bodies[Default]; ok {
		names = append(names, Default)
	}
	return append(names, rest...), bodies
}()

func Summary(body string) string {
	for l := range strings.SplitSeq(body, "\n") {
		l = strings.TrimSpace(l)
		if !strings.HasPrefix(l, "#") {
			break
		}
		if l = strings.TrimSpace(strings.TrimPrefix(l, "#")); l == "" {
			continue
		}
		return strings.TrimSpace(strings.TrimPrefix(l, "cc-statusline —"))
	}
	return ""
}
