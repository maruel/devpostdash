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

func parseHTML(t *testing.T, s string) *html.Node {
	n, err := html.Parse(strings.NewReader(s))
	if err != nil {
		t.Fatalf("Failed to parse HTML: %v", err)
	}
	return n
}

func TestTag(t *testing.T) {
	htmlContent := `<html><body><div id="test">Hello</div><p>World</p></body></html>`
	doc := parseHTML(t, htmlContent)

	tests := []struct {
		tagName  string
		expected string
	}{
		{"div", "div"},
		{"p", "p"},
		{"span", ""}, // Should not find
	}

	for _, tt := range tests {
		t.Run(tt.tagName, func(t *testing.T) {
			selector := Tag(tt.tagName)
			found := FirstChild(doc, selector)
			if tt.expected == "" {
				if found != nil {
					t.Errorf("Expected no node with tag %s, but found %s", tt.tagName, found.Data)
				}
			} else {
				if found == nil || found.Data != tt.expected {
					t.Errorf("Expected node with tag %s, but got %v", tt.tagName, found)
				}
			}
		})
	}
}

func TestAttr(t *testing.T) {
	htmlContent := `<html><body><div id="myid" data-value="123">Hello</div><p class="text">World</p></body></html>`
	doc := parseHTML(t, htmlContent)

	tests := []struct {
		key      string
		val      string
		expected string
	}{
		{"id", "myid", "div"},
		{"data-value", "123", "div"},
		{"class", "text", "p"},
		{"id", "nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key+"="+tt.val, func(t *testing.T) {
			selector := Attr(tt.key, tt.val)
			found := FirstChild(doc, selector)
			if tt.expected == "" {
				if found != nil {
					t.Errorf("Expected no node with attr %s=%s, but found %s", tt.key, tt.val, found.Data)
				}
			} else {
				if found == nil || found.Data != tt.expected {
					t.Errorf("Expected node with attr %s=%s, but got %v", tt.key, tt.val, found)
				}
			}
		})
	}
}

func TestClass(t *testing.T) {
	htmlContent := `<html><body><div class="container item">Hello</div><p class="text">World</p></body></html>`
	doc := parseHTML(t, htmlContent)

	tests := []struct {
		className string
		expected  string
	}{
		{"container", "div"},
		{"item", "div"},
		{"text", "p"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.className, func(t *testing.T) {
			selector := Class(tt.className)
			found := FirstChild(doc, selector)
			if tt.expected == "" {
				if found != nil {
					t.Errorf("Expected no node with class %s, but found %s", tt.className, found.Data)
				}
			} else {
				if found == nil || found.Data != tt.expected {
					t.Errorf("Expected node with class %s, but got %v", tt.className, found)
				}
			}
		})
	}
}

func TestID(t *testing.T) {
	htmlContent := `<html><body><div id="uniqueid">Hello</div><p id="anotherid">World</p></body></html>`
	doc := parseHTML(t, htmlContent)

	tests := []struct {
		id       string
		expected string
	}{
		{"uniqueid", "div"},
		{"anotherid", "p"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.id, func(t *testing.T) {
			selector := ID(tt.id)
			found := FirstChild(doc, selector)
			if tt.expected == "" {
				if found != nil {
					t.Errorf("Expected no node with id %s, but found %s", tt.id, found.Data)
				}
			} else {
				if found == nil || found.Data != tt.expected {
					t.Errorf("Expected node with id %s, but got %v", tt.id, found)
				}
			}
		})
	}
}

func TestType(t *testing.T) {
	htmlContent := `<html><body><!-- comment --><p>Text</p></body></html>`
	doc := parseHTML(t, htmlContent)

	tests := []struct {
		nodeType html.NodeType
		expected bool
	}{
		{html.ElementNode, true},
		{html.TextNode, true},
		{html.CommentNode, true},
		{html.DocumentNode, true},
		{html.DoctypeNode, false}, // No doctype in the example
	}

	for _, tt := range tests {
		t.Run(fmt.Sprintf("%d", tt.nodeType), func(t *testing.T) {
			selector := Type(tt.nodeType)
			found := FirstChild(doc, selector)
			if tt.expected {
				if found == nil {
					t.Errorf("Expected to find node of type %d, but found none", tt.nodeType)
				}
			} else {
				if found != nil {
					t.Errorf("Expected no node of type %d, but found one", tt.nodeType)
				}
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

	// Find first span within div
	spanNode := FirstChild(divNode, Tag("span"))
	if spanNode == nil || spanNode.Data != "span" {
		t.Errorf("Expected to find span within div, got %v", spanNode)
	}

	// Find non-existent
	nonExistent := FirstChild(doc, Tag("table"))
	if nonExistent != nil {
		t.Errorf("Expected to find no table, but found %v", nonExistent)
	}
}

func TestNodeAttr(t *testing.T) {
	htmlContent := `<html><body><div id="myid" class="test-class" data-info="value"></div></body></html>`
	doc := parseHTML(t, htmlContent)
	divNode := FirstChild(doc, Tag("div"))

	tests := []struct {
		key      string
		expected string
	}{
		{"id", "myid"},
		{"class", "test-class"},
		{"data-info", "value"},
		{"nonexistent", ""},
	}

	for _, tt := range tests {
		t.Run(tt.key, func(t *testing.T) {
			attrVal := NodeAttr(divNode, tt.key)
			if attrVal != tt.expected {
				t.Errorf("Expected attribute %s to be %s, got %s", tt.key, tt.expected, attrVal)
			}
		})
	}
}

func TestNodeTextContent(t *testing.T) {
	htmlContent := `<html><body><div>Hello <span>World</span>!</div><p>  Another   line.  </p></body></html>`
	doc := parseHTML(t, htmlContent)

	// Test div content
	divNode := FirstChild(doc, Tag("div"))
	if divNode == nil {
		t.Fatal("Could not find div node")
	}
	expectedDivText := "Hello World!"
	if text := NodeTextContent(divNode); text != expectedDivText {
		t.Errorf("Expected div text '%s', got '%s'", expectedDivText, text)
	}

	// Test p content
	pNode := FirstChild(doc, Tag("p"))
	if pNode == nil {
		t.Fatal("Could not find p node")
	}
	expectedPText := "Another line."
	if text := NodeTextContent(pNode); text != expectedPText {
		t.Errorf("Expected p text '%s', got '%s'", expectedPText, text)
	}

	// Test with no text content
	emptyDiv := parseHTML(t, `<div></div>`)
	if text := NodeTextContent(emptyDiv); text != "" {
		t.Errorf("Expected empty string for empty div, got '%s'", text)
	}

	// Test with only whitespace
	whitespaceDiv := parseHTML(t, `<div>   </div>`)
	if text := NodeTextContent(whitespaceDiv); text != "" {
		t.Errorf("Expected empty string for whitespace div, got '%s'", text)
	}
}
