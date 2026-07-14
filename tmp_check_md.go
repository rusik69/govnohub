package main

import (
	"bytes"
	"fmt"

	"github.com/yuin/goldmark"
	"github.com/yuin/goldmark/extension"
	"github.com/yuin/goldmark/parser"
	"github.com/yuin/goldmark/renderer/html"
)

func main() {
	md := goldmark.New(
		goldmark.WithExtensions(extension.GFM),
		goldmark.WithParserOptions(parser.WithAutoHeadingID()),
		goldmark.WithRendererOptions(html.WithHardWraps(), html.WithXHTML()),
	)

	src := `- [ ] unchecked task
- [x] checked task
- [ ] another unchecked`

	var buf bytes.Buffer
	if err := md.Convert([]byte(src), &buf); err != nil {
		panic(err)
	}
	fmt.Println(buf.String())
}
