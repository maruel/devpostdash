// Copyright 2025 Marc-Antoine Ruel. All rights reserved.
// Use of this source code is governed under the Apache License, Version 2.0
// that can be found in the LICENSE file.

// Package dom provides DOM related functions to parse HTML pages.
package dom

import (
	"bytes"
	"fmt"
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

// NodeText returns the text as processed in HTML.
func NodeText(n *html.Node) string {
	buf := bytes.Buffer{}
	for c := range YieldChildren(n, Type(html.TextNode)) {
		buf.WriteString(c.Data)
	}
	return strings.Join(strings.Fields(buf.String()), " ")
}

// NodeMarkdown returns the node's content as a markdown representation.
//
// This is best effort.
func NodeMarkdown(n *html.Node) string {
	buf := bytes.Buffer{}
	nodeMarkdownRecursive(&buf, n)
	return buf.String()
}

//

// getDirectTextContent returns the direct text content of a node, without recursing into child elements.
func getDirectTextContent(n *html.Node) string {
	var buf bytes.Buffer
	for c := n.FirstChild; c != nil; c = c.NextSibling {
		if c.Type == html.TextNode {
			buf.WriteString(c.Data)
		}
	}
	return buf.String()
}

// getMarkdownContent returns the node's content as a markdown representation.
func getMarkdownContent(n *html.Node) string {
	buf := bytes.Buffer{}
	nodeMarkdownRecursive(&buf, n)
	return buf.String()
}

func nodeMarkdownRecursive(buf *bytes.Buffer, n *html.Node) {
	switch n.Type {
	case html.TextNode:
		buf.WriteString(n.Data)
	case html.ElementNode:
		switch n.Data {
		case "a":
			href := NodeAttr(n, "href")
			buf.WriteString("[")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("](")
			buf.WriteString(href)
			buf.WriteString(")")
		case "strong", "b":
			buf.WriteString("**")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("**")
		case "em", "i":
			buf.WriteString("*")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("*")
		case "img":
			buf.WriteString("![")
			// TODO: Handle when there's no alt. Use figcaption.
			buf.WriteString(NodeAttr(n, "alt"))
			buf.WriteString("](")
			buf.WriteString(NodeAttr(n, "src"))
			buf.WriteString(")")
		case "p":
			buf.WriteString("\n\n")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("\n\n")
		case "br":
			buf.WriteString("\n")
		case "h1":
			buf.WriteString("\n# ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("\n\n")
		case "h2":
			buf.WriteString("\n## ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("\n\n")
		case "h3":
			buf.WriteString("\n### ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("\n\n")
		case "h4":
			buf.WriteString("\n#### ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("\n\n")
		case "h5":
			buf.WriteString("\n##### ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("\n\n")
		case "h6":
			buf.WriteString("\n###### ")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("\n\n")
		case "ul":
			buf.WriteString("\n")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "li" {
					buf.WriteString("- ")
					for liChild := c.FirstChild; liChild != nil; liChild = liChild.NextSibling {
						nodeMarkdownRecursive(buf, liChild)
					}
					buf.WriteString("\n")
				}
			}
		case "ol":
			buf.WriteString("\n")
			itemNum := 1
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode && c.Data == "li" {
					buf.WriteString(fmt.Sprintf("%d. ", itemNum))
					for liChild := c.FirstChild; liChild != nil; liChild = liChild.NextSibling {
						nodeMarkdownRecursive(buf, liChild)
					}
					buf.WriteString("\n")
					itemNum++
				}
			}
		case "code":
			buf.WriteString("`")
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
			buf.WriteString("`")
		case "pre":
			buf.WriteString("\n```\n")
			buf.WriteString(getDirectTextContent(n))
			buf.WriteString("\n```\n")
		case "table":
			buf.WriteString("\n")
			// Collect all rows first, separating header from body rows
			var headerCells []string
			var bodyRows [][]string
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				if c.Type == html.ElementNode {
					if c.Data == "thead" {
						for th := c.FirstChild; th != nil; th = th.NextSibling {
							if th.Type == html.ElementNode && th.Data == "tr" {
								for cell := th.FirstChild; cell != nil; cell = cell.NextSibling {
									if cell.Type == html.ElementNode && cell.Data == "th" {
										headerCells = append(headerCells, getMarkdownContent(cell))
									}
								}
							}
						}
					} else if c.Data == "tbody" || c.Data == "tr" { // Handle tbody or direct tr children
						var rowsToProcess *html.Node
						if c.Data == "tbody" {
							rowsToProcess = c.FirstChild
						} else { // c.Data == "tr"
							rowsToProcess = c
						}
						for tr := rowsToProcess; tr != nil; tr = tr.NextSibling {
							if tr.Type == html.ElementNode && tr.Data == "tr" {
								var rowData []string
								for cell := tr.FirstChild; cell != nil; cell = cell.NextSibling {
									if cell.Type == html.ElementNode && (cell.Data == "td" || cell.Data == "th") {
										rowData = append(rowData, getMarkdownContent(cell))
									}
								}
								bodyRows = append(bodyRows, rowData)
							}
							if c.Data == "tr" { // If we processed a direct tr, break after it
								break
							}
						}
					}
				}
			}

			// Print header if present
			if len(headerCells) > 0 {
				buf.WriteString("| ")
				buf.WriteString(strings.Join(headerCells, " | "))
				buf.WriteString(" |\n")
				buf.WriteString("|")
				for range len(headerCells) {
					buf.WriteString(" --- |")
				}
				buf.WriteString("\n")
			}
			// Print body rows
			for _, rowData := range bodyRows {
				buf.WriteString("| ")
				buf.WriteString(strings.Join(rowData, " | "))
				buf.WriteString(" |\n")
			}
			buf.WriteString("\n")
		default:
			// For other elements, just recurse into children
			for c := n.FirstChild; c != nil; c = c.NextSibling {
				nodeMarkdownRecursive(buf, c)
			}
		}
	case html.DocumentNode, html.DoctypeNode, html.CommentNode:
		// Do nothing for these types, just recurse into children
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			nodeMarkdownRecursive(buf, c)
		}
	default:
		panic("TODO")
	}
}
