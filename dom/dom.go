// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package dom provides DOM related functions to parse HTML pages.
package dom

import (
	"bytes"
	"iter"
	"slices"
	"strings"

	"golang.org/x/net/html"
)

// Selector is a filter to select nodes in an DOM tree.
type Selector func(*html.Node) bool

// Tag selects element nodes by tag name.
func Tag(tagName string) Selector {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && n.Data == tagName
	}
}

// Attr selects element nodes by attribute name and value.
func Attr(key, val string) Selector {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && NodeAttr(n, key) == val
	}
}

// Class selects element nodes by class name.
func Class(className string) Selector {
	return func(n *html.Node) bool {
		return n.Type == html.ElementNode && slices.Contains(strings.Split(NodeAttr(n, "class"), " "), className)
	}
}

// ID selects nodes by id.
func ID(id string) Selector {
	return Attr("id", id)
}

// Type selects nodes by type.
func Type(t html.NodeType) Selector {
	return func(n *html.Node) bool { return n.Type == t }
}

// YieldChildren travel the tree with the filter specified, traversing depth first.
func YieldChildren(n *html.Node, filters ...Selector) iter.Seq[*html.Node] {
	return func(yield func(*html.Node) bool) {
		ok := true
		for _, f := range filters {
			if !f(n) {
				ok = false
				break
			}
		}
		if ok && !yield(n) {
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			for m := range YieldChildren(c, filters...) {
				if !yield(m) {
					return
				}
			}
		}
	}
}

// FirstChild travel the tree with the filter specified and returns the first node found.
func FirstChild(n *html.Node, filters ...Selector) *html.Node {
	for n := range YieldChildren(n, filters...) {
		return n
	}
	return nil
}

// NodeAttr returns the attribute of an html node if it exists.
func NodeAttr(n *html.Node, key string) string {
	for _, a := range n.Attr {
		if a.Key == key {
			return a.Val
		}
	}
	return ""
}

// NodeTextContent returns the text as processed in HTML.
func NodeTextContent(n *html.Node) string {
	buf := bytes.Buffer{}
	for c := range YieldChildren(n, Type(html.TextNode)) {
		buf.WriteString(c.Data)
	}
	return strings.TrimSpace(buf.String())
}
