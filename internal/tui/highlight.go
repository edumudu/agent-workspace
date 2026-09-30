package tui

import (
	"path"
	"strings"

	"github.com/alecthomas/chroma/v2"

	"github.com/giovaniif/agent-workspace/internal/domain"
)

// tok is a run of text in one syntax color; an empty color is the theme's
// text color.
type tok struct {
	text  string
	color string
}

// highlight colors each line of a file's hunks with chroma. The lines are
// lexed as one text so multi-line tokens such as block comments keep their
// color; the old and new sides are interleaved, which is close enough for a
// diff.
// syntaxColors maps token categories to theme colors. It stands in for
// chroma's styles package, whose init parses every bundled style and would
// cost each `agentws hook` run about 3 ms.
type syntaxColors struct {
	keyword, name, function, str, number, comment, operator string
}

func newSyntaxColors(t Theme) syntaxColors {
	return syntaxColors{keyword: t.Mauve, name: t.Text, function: t.Blue, str: t.Green, number: t.Peach, comment: t.Overlay, operator: t.Teal}
}

func (c syntaxColors) color(tt chroma.TokenType) string {
	switch {
	case tt.InCategory(chroma.Comment):
		return c.comment
	case tt.InCategory(chroma.Keyword):
		return c.keyword
	case tt == chroma.NameFunction || tt == chroma.NameClass || tt == chroma.NameBuiltin:
		return c.function
	case tt.InSubCategory(chroma.LiteralString):
		return c.str
	case tt.InSubCategory(chroma.LiteralNumber):
		return c.number
	case tt.InCategory(chroma.Operator):
		return c.operator
	}
	return ""
}

func highlight(syntax syntaxColors, filename string, lines []domain.DiffLine) (out [][]tok) {
	texts := make([]string, len(lines))
	for i, l := range lines {
		texts[i] = cleanText(l.Text)
	}
	plain := func() [][]tok {
		p := make([][]tok, len(texts))
		for i, t := range texts {
			p[i] = []tok{{text: t}}
		}
		return p
	}
	// why: chroma panics on some malformed input; plain text beats a crashed TUI.
	defer func() {
		if recover() != nil {
			out = plain()
		}
	}()
	lexer := matchLexer(path.Base(filename))
	if lexer == nil {
		return plain()
	}
	it, err := chroma.Coalesce(lexer).Tokenise(nil, strings.Join(texts, "\n"))
	if err != nil {
		return plain()
	}
	out = make([][]tok, len(lines))
	row := 0
	for t := it(); t != chroma.EOF && row < len(out); t = it() {
		color := syntax.color(t.Type)
		parts := strings.Split(t.Value, "\n")
		for j, p := range parts {
			if j > 0 {
				row++
				if row >= len(out) {
					break
				}
			}
			if p != "" {
				out[row] = append(out[row], tok{text: p, color: color})
			}
		}
	}
	return out
}
