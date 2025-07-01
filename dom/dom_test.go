// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

package dom

import (
	"fmt"
	"strings"
	"testing"

	"golang.org/x/net/html"
)

func TestTag(t *testing.T) {
	htmlContent := `<html><body><div id="test">Hello</div><p>World</p></body></html>`
	doc := parseHTML(t, htmlContent)
	tests := []struct {
		tagName string
		want    string
	}{
		{"div", "div"},
		{"p", "p"},
		{"span", ""},
	}
	for _, tt := range tests {
		t.Run(tt.tagName, func(t *testing.T) {
			if found := FirstChild(doc, Tag(tt.tagName)); tt.want == "" {
				if found != nil {
					t.Errorf("Expected no node with tag %s, but found %s", tt.tagName, found.Data)
				}
			} else if found == nil || found.Data != tt.want {
				t.Errorf("Expected node with tag %s, but got %v", tt.tagName, found)
			}
		})
	}
}

func TestAttr(t *testing.T) {
	htmlContent := `<html><body><div id="myid" data-value="123">Hello</div><p class="text">World</p></body></html>`
	doc := parseHTML(t, htmlContent)
	tests := []struct {
		key  string
		val  string
		want string
	}{
		{"id", "myid", "div"},
		{"data-value", "123", "div"},
		{"class", "text", "p"},
		{"id", "nonexistent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.key+"="+tt.val, func(t *testing.T) {
			if found := FirstChild(doc, Attr(tt.key, tt.val)); tt.want == "" {
				if found != nil {
					t.Errorf("Expected no node with attr %s=%s, but found %s", tt.key, tt.val, found.Data)
				}
			} else if found == nil || found.Data != tt.want {
				t.Errorf("Expected node with attr %s=%s, but got %v", tt.key, tt.val, found)
			}
		})
	}
}

func TestClass(t *testing.T) {
	htmlContent := `<html><body><div class="container item">Hello</div><p class="text">World</p></body></html>`
	doc := parseHTML(t, htmlContent)
	tests := []struct {
		className string
		want      string
	}{
		{"container", "div"},
		{"item", "div"},
		{"text", "p"},
		{"nonexistent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.className, func(t *testing.T) {
			if found := FirstChild(doc, Class(tt.className)); tt.want == "" {
				if found != nil {
					t.Errorf("Expected no node with class %s, but found %s", tt.className, found.Data)
				}
			} else if found == nil || found.Data != tt.want {
				t.Errorf("Expected node with class %s, but got %v", tt.className, found)
			}
		})
	}
}

func TestID(t *testing.T) {
	htmlContent := `<html><body><div id="uniqueid">Hello</div><p id="anotherid">World</p></body></html>`
	doc := parseHTML(t, htmlContent)
	tests := []struct {
		id   string
		want string
	}{
		{"uniqueid", "div"},
		{"anotherid", "p"},
		{"nonexistent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			if found := FirstChild(doc, ID(tt.id)); tt.want == "" {
				if found != nil {
					t.Errorf("Expected no node with id %s, but found %s", tt.id, found.Data)
				}
			} else if found == nil || found.Data != tt.want {
				t.Errorf("Expected node with id %s, but got %v", tt.id, found)
			}
		})
	}
}

func TestType(t *testing.T) {
	htmlContent := `<html><body><!-- comment --><p>Text</p></body></html>`
	doc := parseHTML(t, htmlContent)
	tests := []struct {
		nodeType html.NodeType
		want     bool
	}{
		{html.ElementNode, true},
		{html.TextNode, true},
		{html.CommentNode, true},
		{html.DocumentNode, true},
		{html.DoctypeNode, false}, // No doctype in the example
	}
	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.nodeType), func(t *testing.T) {
			if found := FirstChild(doc, Type(tt.nodeType)); tt.want {
				if found == nil {
					t.Errorf("Expected to find node of type %d, but found none", tt.nodeType)
				}
			} else if found != nil {
				t.Errorf("Expected no node of type %d, but found one", tt.nodeType)
			}
		})
	}
}

func TestYieldChildren(t *testing.T) {
	htmlContent := `<html><body><div><span>1</span><p>2</p></div><span>3</span></body></html>`
	doc := parseHTML(t, htmlContent)
	var foundTags []string
	for n := range YieldChildren(doc, Type(html.ElementNode)) {
		foundTags = append(foundTags, n.Data)
	}
	expectedTags := []string{"html", "head", "body", "div", "span", "p", "span"}
	if len(foundTags) != len(expectedTags) {
		t.Fatalf("Expected %d tags, got %d: %v", len(expectedTags), len(foundTags), foundTags)
	}
	for i, tag := range foundTags {
		if tag != expectedTags[i] {
			t.Errorf("Expected tag %s at index %d, got %s", expectedTags[i], i, tag)
		}
	}

	// Test with filter
	foundDivs := 0
	for n := range YieldChildren(doc, Tag("div")) {
		if n.Data == "div" {
			foundDivs++
		}
	}
	if foundDivs != 1 {
		t.Errorf("Expected 1 div, got %d", foundDivs)
	}
}

func TestFirstChild(t *testing.T) {
	htmlContent := `<html><body><div><span>1</span><p>2</p></div><span>3</span></body></html>`
	doc := parseHTML(t, htmlContent)

	// Find first div
	divNode := FirstChild(doc, Tag("div"))
	if divNode == nil || divNode.Data != "div" {
		t.Errorf("Expected to find div, got %v", divNode)
	}

	if spanNode := FirstChild(divNode, Tag("span")); spanNode == nil || spanNode.Data != "span" {
		t.Errorf("Expected to find span within div, got %v", spanNode)
	}

	if nonExistent := FirstChild(doc, Tag("table")); nonExistent != nil {
		t.Errorf("Expected to find no table, but found %v", nonExistent)
	}
}

func TestNodeAttr(t *testing.T) {
	htmlContent := `<html><body><div id="myid" class="test-class" data-info="value"></div></body></html>`
	divNode := FirstChild(parseHTML(t, htmlContent), Tag("div"))
	tests := []struct {
		key  string
		want string
	}{
		{"id", "myid"},
		{"class", "test-class"},
		{"data-info", "value"},
		{"nonexistent", ""},
	}
	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			if got := NodeAttr(divNode, tt.key); got != tt.want {
				t.Errorf("Expected attribute %s to be %s, got %s", tt.key, tt.want, got)
			}
		})
	}
}

func TestNodeText(t *testing.T) {
	htmlContent := `<html><body><div>Hello <span>World</span>!</div><p>  Another   line.  </p></body></html>`
	doc := parseHTML(t, htmlContent)

	if n := FirstChild(doc, Tag("body")); n == nil {
		t.Fatal("Could not find body node")
	} else {
		want := "Hello World! Another line."
		if got := NodeText(n); got != want {
			t.Errorf("Expected div text '%s', got '%s'", want, got)
		}
	}
	if n := FirstChild(doc, Tag("div")); n == nil {
		t.Fatal("Could not find div node")
	} else {
		want := "Hello World!"
		if got := NodeText(n); got != want {
			t.Errorf("Expected div text '%s', got '%s'", want, got)
		}
	}
	if n := FirstChild(doc, Tag("p")); n == nil {
		t.Fatal("Could not find p node")
	} else {
		want := "Another line."
		if got := NodeText(n); got != want {
			t.Errorf("Expected p text '%s', got '%s'", want, got)
		}
	}

	if got := NodeText(parseHTML(t, `<div></div>`)); got != "" {
		t.Errorf("Expected empty string for empty div, got '%s'", got)
	}

	if got := NodeText(parseHTML(t, `<div>   </div>`)); got != "" {
		t.Errorf("Expected empty string for whitespace div, got '%s'", got)
	}
}

func TestNodeMarkdown(t *testing.T) {
	tests := []struct {
		name string
		html string
		want string
	}{
		{
			name: "Link",
			html: `<a href="https://example.com">Example Link</a>`,
			want: "[Example Link](https://example.com)",
		},
		{
			name: "Image",
			html: `<img src="image.jpg" alt="Description of image">`,
			want: "![Description of image](image.jpg)",
		},
		{
			name: "Bold",
			html: `<b>Bold Text</b><strong>Strong Text</strong>`,
			want: "**Bold Text****Strong Text**",
		},
		{
			name: "Italic",
			html: `<i>Italic Text</i><em>Emphasized Text</em>`,
			want: "*Italic Text**Emphasized Text*",
		},
		{
			name: "Code",
			html: `This is some <code>inline code</code>.`,
			want: "This is some `inline code`.",
		},
		{
			name: "Preformatted Text (Code Block)",
			html: "<pre>func main() {\n    fmt.Println(\"Hello, World!\")\n}</pre>",
			want: "\n```\nfunc main() {\n    fmt.Println(\"Hello, World!\")\n}\n```\n",
		},
		{
			name: "Table",
			html: `<table><thead><tr><th>Header 1</th><th>Header 2</th></tr></thead><tbody><tr><td>Row 1 Col 1</td><td>Row 1 Col 2</td></tr><tr><td>Row 2 Col 1</td><td>Row 2 Col 2</td></tr></tbody></table>`,
			want: "\n| Header 1 | Header 2 |\n| --- | --- |\n| Row 1 Col 1 | Row 1 Col 2 |\n| Row 2 Col 1 | Row 2 Col 2 |\n\n",
		},
		{
			name: "Paragraph and Line Break",
			html: `<p>Line 1<br>Line 2</p>`,
			want: "\n\nLine 1\nLine 2\n\n",
		},
		{
			name: "Mixed Content",
			html: `<h1>Title</h1><p>This is a <b>test</b> with an <a href="#">inline link</a> and <i>some italic text</i>.</p>`,
			want: "\n# Title\n\n\n\nThis is a **test** with an [inline link](#) and *some italic text*.\n\n",
		},
		{
			name: "Unordered List",
			html: `<ul><li>Item 1</li><li>Item 2</li></ul>`,
			want: "\n- Item 1\n- Item 2\n",
		},
		{
			name: "Ordered List",
			html: `<ol><li>First item</li><li>Second item</li></ol>`,
			want: "\n1. First item\n2. Second item\n",
		},
		{
			name: "Table with strong, em, without tbody",
			html: `<table><thead><tr><th>**Header 1**</th><th>*Header 2*</th></tr></thead><tr><td>Row 1 Col 1</td><td>Row 1 Col 2</td></tr></table>`,
			want: "\n| **Header 1** | *Header 2* |\n| --- | --- |\n| Row 1 Col 1 | Row 1 Col 2 |\n\n",
		},
	}
	for _, tt := range tests {
		t.Run(tt.name, func(t *testing.T) {
			doc := parseHTML(t, tt.html)
			// We need to get the body or a relevant root node for markdown conversion
			bodyNode := FirstChild(doc, Tag("body"))
			if bodyNode == nil {
				bodyNode = doc // Fallback to document if no body (e.g., fragment HTML)
			}
			if got := NodeMarkdown(bodyNode); got != tt.want {
				t.Errorf("NodeMarkdown() for %s\nGot:\n%q\nWant:\n%q", tt.name, got, tt.want)
			}
		})
	}
}

//

func parseHTML(t *testing.T, s string) *html.Node {
	n, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}
	return n
}
