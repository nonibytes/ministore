package sqlbuilder

import "sort"

type PlaceholderStyle int

const (
	PlaceholderQuestion PlaceholderStyle = iota
	PlaceholderDollar
)

type Builder struct {
	Style PlaceholderStyle
	args  []any
}

func New(style PlaceholderStyle) *Builder {
	return &Builder{Style: style, args: make([]any, 0)}
}

func (b *Builder) Arg(v any) string {
	b.args = append(b.args, v)
	switch b.Style {
	case PlaceholderDollar:
		return "$" + itoa(len(b.args))
	default:
		return "?"
	}
}

func (b *Builder) Args() []any { return b.args }
func (b *Builder) Len() int    { return len(b.args) }

// itoa converts int to string without fmt overhead
func itoa(n int) string {
	if n == 0 {
		return "0"
	}
	var buf [32]byte
	i := len(buf)
	for n > 0 {
		i--
		buf[i] = byte('0' + (n % 10))
		n /= 10
	}
	return string(buf[i:])
}

// SortStrings sorts a slice of strings in place
func SortStrings(s []string) {
	sort.Strings(s)
}
