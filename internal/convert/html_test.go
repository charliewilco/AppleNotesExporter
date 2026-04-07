package convert

import (
	"strings"
	"testing"
)

const onePixelPNG = "iVBORw0KGgoAAAANSUhEUgAAAAEAAAABCAYAAAAfFcSJAAAADUlEQVQIHWP4////fwAJ+wP+Z7S3GQAAAABJRU5ErkJggg=="

func TestHTMLToMarkdownConvertsTablesAndImages(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		`<div><h1>Heading</h1></div>`,
		`<div>Hello <b>world</b></div>`,
		`<div><img src="data:image/png;base64,` + onePixelPNG + `" alt="Screenshot"></div>`,
		`<div><object><table><tbody><tr><td><div><b>Name</b></div></td><td><div><b>Value</b></div></td></tr><tr><td><div>One</div></td><td><div>Two</div></td></tr></tbody></table></object></div>`,
	}, "")

	result, err := HTMLToMarkdown(input, Options{
		AssetDir:    "assets",
		AssetPrefix: "sample",
	})
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}

	if len(result.Assets) != 1 {
		t.Fatalf("expected 1 asset, got %d", len(result.Assets))
	}

	if result.Assets[0].RelativePath != "assets/sample-001.png" {
		t.Fatalf("unexpected asset path %q", result.Assets[0].RelativePath)
	}

	for _, snippet := range []string{
		"# Heading",
		"Hello **world**",
		"![Screenshot](assets/sample-001.png)",
		"| **Name** | **Value** |",
		"| One | Two |",
	} {
		if !strings.Contains(result.Markdown, snippet) {
			t.Fatalf("markdown missing snippet %q:\n%s", snippet, result.Markdown)
		}
	}
}

func TestHTMLToMarkdownConvertsListsQuotesAndCode(t *testing.T) {
	t.Parallel()

	input := strings.Join([]string{
		`<blockquote><div>Quoted <i>text</i></div></blockquote>`,
		`<ul><li><input type="checkbox" checked="checked">Ship it<ul><li>Nested item</li></ul></li></ul>`,
		`<div style="font-family: Menlo">let answer = 42</div>`,
	}, "")

	result, err := HTMLToMarkdown(input, Options{
		AssetDir:    "assets",
		AssetPrefix: "sample",
	})
	if err != nil {
		t.Fatalf("HTMLToMarkdown returned error: %v", err)
	}

	for _, snippet := range []string{
		"> Quoted *text*",
		"- [x] Ship it",
		"  - Nested item",
		"```\nlet answer = 42\n```",
	} {
		if !strings.Contains(result.Markdown, snippet) {
			t.Fatalf("markdown missing snippet %q:\n%s", snippet, result.Markdown)
		}
	}
}
