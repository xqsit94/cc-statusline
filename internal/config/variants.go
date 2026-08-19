package config

type KeyValue struct{ Key, Value string }

type Variant []KeyValue

var Variants = derivedVariants()

type accessor struct {
	segment string
	get     func(*Config) string
	set     func(*Config, string)
}

var (
	accessorByKey  = map[string]accessor{}
	keysBySegment  = map[string][]string{}
	segmentOrdered []string
)

func init() {
	for _, d := range SegmentDefs {
		for _, k := range d.PresentationKeys() {
			key := k.Path(d.Name)
			accessorByKey[key] = accessor{segment: d.Name, get: k.Get, set: k.Set}
			if _, seen := keysBySegment[d.Name]; !seen {
				segmentOrdered = append(segmentOrdered, d.Name)
			}
			keysBySegment[d.Name] = append(keysBySegment[d.Name], key)
		}
	}
}

func SegmentKeys(segment string) []string { return keysBySegment[segment] }

func VariantOf(segment string, s Segments) Variant {
	c := Config{Segments: s}
	var v Variant
	for _, key := range keysBySegment[segment] {
		v = append(v, KeyValue{key, accessorByKey[key].get(&c)})
	}
	return v
}

func ApplyVariant(v Variant, s *Segments) {
	c := Config{Segments: *s}
	for _, kv := range v {
		a, ok := accessorByKey[kv.Key]
		if !ok {
			continue
		}
		a.set(&c, kv.Value)
	}
	*s = c.Segments
}

func IndexOfVariant(vs []Variant, segment string, s Segments) int {
	cur := VariantOf(segment, s)
	for i, v := range vs {
		if sameAssignment(v, cur) {
			return i
		}
	}
	return -1
}

func sameAssignment(a, b Variant) bool {
	if len(a) != len(b) {
		return false
	}
	for _, kv := range a {
		if got, ok := b.value(kv.Key); !ok || got != kv.Value {
			return false
		}
	}
	return true
}

func (v Variant) value(key string) (string, bool) {
	for _, kv := range v {
		if kv.Key == key {
			return kv.Value, true
		}
	}
	return "", false
}

func Changed(a, b Segments) []KeyValue {
	ca, cb := Config{Segments: a}, Config{Segments: b}
	var out []KeyValue
	for _, segment := range segmentOrdered {
		for _, key := range keysBySegment[segment] {
			acc := accessorByKey[key]
			if got := acc.get(&ca); got != acc.get(&cb) {
				out = append(out, KeyValue{key, got})
			}
		}
	}
	return out
}
