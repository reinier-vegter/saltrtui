// Package target resolves an ID-based candidate set from an accepted-key snapshot.
package target

import (
	"errors"
	"regexp"
	"sort"
	"strings"
	"unicode/utf8"
)

type Mode string

const (
	List Mode = "list"
	Glob Mode = "glob"
)

type Preview struct {
	Mode       Mode
	Expression string
	IDs        []string
}

func validID(id string) bool {
	return id != "" && !strings.HasPrefix(id, "-") && !strings.ContainsAny(id, ",\x00\r\n")
}

// Resolve uses the supplied fresh accepted-key inventory. List members must
// exist in that inventory; patterns use fnmatch-style matching of the entire
// minion ID, including '/' characters (unlike filepath.Match).
func Resolve(mode Mode, expression string, accepted []string) (Preview, error) {
	result := Preview{Mode: mode, Expression: strings.TrimSpace(expression)}
	if result.Expression == "" || len(result.Expression) > 8192 || !utf8.ValidString(result.Expression) || strings.ContainsAny(result.Expression, "\x00\r\n") {
		return result, errors.New("enter a valid target expression (up to 8192 bytes)")
	}
	known := make(map[string]bool, len(accepted))
	for _, id := range accepted {
		known[id] = true
	}
	seen := make(map[string]bool)
	switch mode {
	case List:
		for _, part := range strings.Split(result.Expression, ",") {
			id := strings.TrimSpace(part)
			if !validID(id) {
				return result, errors.New("specific minions require comma-separated IDs without empty or ambiguous entries")
			}
			if !known[id] {
				return result, errors.New("minion " + id + " has no accepted key in this snapshot")
			}
			if !seen[id] {
				result.IDs = append(result.IDs, id)
				seen[id] = true
			}
		}
	case Glob:
		matcher, err := compileGlob(result.Expression)
		if err != nil {
			return result, err
		}
		for _, id := range accepted {
			if !validID(id) {
				continue // An ambiguous ID cannot be safely handed to list targeting.
			}
			if matcher.MatchString(id) && !seen[id] {
				result.IDs = append(result.IDs, id)
				seen[id] = true
			}
		}
	default:
		return result, errors.New("unsupported matcher")
	}
	sort.Strings(result.IDs)
	return result, nil
}

// Python fnmatchcase-style shell globbing: slash is an ordinary character;
// unmatched '[' is literal and [!a-z] negates a character class.
func compileGlob(pattern string) (*regexp.Regexp, error) {
	var b strings.Builder
	b.WriteString("(?s:^")
	for i := 0; i < len(pattern); {
		advance := 1
		switch pattern[i] {
		case '*':
			b.WriteString(".*")
		case '?':
			b.WriteByte('.')
		case '[':
			j := i + 1
			if j < len(pattern) && pattern[j] == '!' {
				j++
			}
			if j < len(pattern) && pattern[j] == ']' {
				j++
			}
			for j < len(pattern) && pattern[j] != ']' {
				j++
			}
			if j == len(pattern) {
				b.WriteString(`\[`)
			} else {
				class := pattern[i+1 : j]
				if strings.HasPrefix(class, "!") {
					class = "^" + class[1:]
				}
				b.WriteByte('[')
				b.WriteString(class)
				b.WriteByte(']')
				i = j
			}
		default:
			_, advance = utf8.DecodeRuneInString(pattern[i:])
			b.WriteString(regexp.QuoteMeta(pattern[i : i+advance]))
		}
		i += advance
	}
	b.WriteString("$)")
	matcher, err := regexp.Compile(b.String())
	if err != nil {
		return nil, errors.New("invalid ID pattern")
	}
	return matcher, nil
}

// Suggestions operate on the last list entry or the literal prefix of a
// pattern; they do not expand or change the actual target expression.
func Suggestions(mode Mode, expression string, accepted []string) []string {
	fragment := expression
	if mode == List {
		if i := strings.LastIndex(fragment, ","); i >= 0 {
			fragment = fragment[i+1:]
		}
	} else if i := strings.IndexAny(fragment, "*?["); i >= 0 {
		fragment = fragment[:i]
	}
	fragment = strings.ToLower(strings.TrimSpace(fragment))
	if fragment == "" {
		return nil
	}
	var out []string
	for _, id := range accepted {
		if validID(id) && strings.HasPrefix(strings.ToLower(id), fragment) {
			out = append(out, id)
		}
	}
	sort.Strings(out)
	if len(out) > 6 {
		out = out[:6]
	}
	return out
}
