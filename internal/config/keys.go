package config

type ColorKey struct {
	Name string
	Get  func(*Config) string
	Set  func(*Config, string)
}

var ColorKeys = derivedColorKeys()

var colorByName = func() map[string]ColorKey {
	m := make(map[string]ColorKey, len(ColorKeys))
	for _, k := range ColorKeys {
		m[k.Name] = k
	}
	return m
}()

func (c *Config) Color(name string) (string, bool) {
	k, ok := colorByName[name]
	if !ok {
		return "", false
	}
	return k.Get(c), true
}

type FormatKey struct {
	Key          string
	Segment      string
	Placeholders []string
	Get          func(*Config) string
	Set          func(*Config, string)
}

var FormatKeys = derivedFormatKeys()

type TimeKey struct {
	Key     string
	Segment string
	Get     func(*Config) string
	Set     func(*Config, string)
}

var TimeKeys = derivedTimeKeys()
