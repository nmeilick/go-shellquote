package shellquote

import (
	"bytes"
	"errors"
	"strings"
	"unicode/utf8"
)

var (
	UnterminatedSingleQuoteError = errors.New("Unterminated single-quoted string")
	UnterminatedDoubleQuoteError = errors.New("Unterminated double-quoted string")
	UnterminatedEscapeError      = errors.New("Unterminated backslash-escape")
)

const (
	DefaultSplitChars = " \n\t"
)

type SplitOptions struct {
	SplitChars string
	Limit      int
}

var defaultSplitOptions = SplitOptions{
	SplitChars: DefaultSplitChars,
	//SkipAdjacent: false,
	Limit: -1,
}

var (
	singleChar        = '\''
	doubleChar        = '"'
	escapeChar        = '\\'
	doubleEscapeChars = "$`\"\n\\"
)

// SplitWithOptions splits a string according to /bin/sh's word-splitting rules and
// the options given.
// It supports backslash-escapes, single-quotes, and double-quotes. Notably it does
// not support the $'' style of quoting. It also doesn't attempt to perform any
// other sort of expansion, including brace expansion, shell expansion, or
// pathname expansion.
//
// If the given input has an unterminated quoted string or ends in a
// backslash-escape, one of UnterminatedSingleQuoteError,
// UnterminatedDoubleQuoteError, or UnterminatedEscapeError is returned.
func SplitWithOptions(input string, opts *SplitOptions) (words []string, err error) {
	if opts == nil {
		opts = &defaultSplitOptions
	}

	splitChars := opts.SplitChars
	if len(splitChars) == 0 {
		splitChars = DefaultSplitChars
	}

	switch opts.Limit {
	case 0:
		words = []string{}
		return
	case 1:
		words = []string{}
		input = strings.TrimLeft(strings.TrimRight(input, splitChars), splitChars)
		if len(input) > 0 {
			words = append(words, input)
		}
		return
	}

	var buf bytes.Buffer
	words = make([]string, 0)

	for len(input) > 0 {
		// skip any splitChars at the start
		c, l := utf8.DecodeRuneInString(input)
		if strings.ContainsRune(splitChars, c) {
			input = input[l:]
			continue
		} else if c == escapeChar {
			// Look ahead for escaped newline so we can skip over it
			next := input[l:]
			if len(next) == 0 {
				err = UnterminatedEscapeError
				return
			}
			c2, l2 := utf8.DecodeRuneInString(next)
			if c2 == '\n' {
				input = next[l2:]
				continue
			}
		}

		var word string
		word, input, err = splitWord(input, &buf, opts)
		if err != nil {
			return
		}
		words = append(words, word)
		if opts.Limit == len(words)+1 {
			input = strings.TrimSpace(input)
			if len(input) > 0 {
				words = append(words, input)
			}
			return
		}
	}
	return
}

func Split(input string) (words []string, err error) {
	return SplitWithOptions(input, &defaultSplitOptions)
}

func SplitN(input string, n int) (words []string, err error) {
	opts := *&defaultSplitOptions
	opts.Limit = n
	return SplitWithOptions(input, &opts)
}

func splitWord(input string, buf *bytes.Buffer, opts *SplitOptions) (word string, remainder string, err error) {
	buf.Reset()

raw:
	{
		cur := input
		for len(cur) > 0 {
			c, l := utf8.DecodeRuneInString(cur)
			cur = cur[l:]
			if c == singleChar {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				input = cur
				goto single
			} else if c == doubleChar {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				input = cur
				goto double
			} else if c == escapeChar {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				input = cur
				goto escape
			} else if strings.ContainsRune(opts.SplitChars, c) {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				return buf.String(), cur, nil
			}
		}
		if len(input) > 0 {
			buf.WriteString(input)
			input = ""
		}
		goto done
	}

escape:
	{
		if len(input) == 0 {
			return "", "", UnterminatedEscapeError
		}
		c, l := utf8.DecodeRuneInString(input)
		if c == '\n' {
			// a backslash-escaped newline is elided from the output entirely
		} else {
			buf.WriteString(input[:l])
		}
		input = input[l:]
	}
	goto raw

single:
	{
		i := strings.IndexRune(input, singleChar)
		if i == -1 {
			return "", "", UnterminatedSingleQuoteError
		}
		buf.WriteString(input[0:i])
		input = input[i+1:]
		goto raw
	}

double:
	{
		cur := input
		for len(cur) > 0 {
			c, l := utf8.DecodeRuneInString(cur)
			cur = cur[l:]
			if c == doubleChar {
				buf.WriteString(input[0 : len(input)-len(cur)-l])
				input = cur
				goto raw
			} else if c == escapeChar {
				// bash only supports certain escapes in double-quoted strings
				c2, l2 := utf8.DecodeRuneInString(cur)
				cur = cur[l2:]
				if strings.ContainsRune(doubleEscapeChars, c2) {
					buf.WriteString(input[0 : len(input)-len(cur)-l-l2])
					if c2 == '\n' {
						// newline is special, skip the backslash entirely
					} else {
						buf.WriteRune(c2)
					}
					input = cur
				}
			}
		}
		return "", "", UnterminatedDoubleQuoteError
	}

done:
	return buf.String(), input, nil
}
