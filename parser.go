package main

import (
	"strings"
	"net/url"
)

func ExtractLinks(html string, base string) []string {
	var links []string
	start := 0

	for {
		startLink := strings.Index(html[start:], "<a href=\"")
		if startLink == -1 {
			break
		}

		startLink += start + len("<a href=\"")
		endQuote := strings.Index(html[startLink:], "\"")
		if endQuote == -1 {
			break
		}

		rawLink := html[startLink : startLink+endQuote]
		start = startLink + endQuote

		parsedBase, err := url.Parse(base)
		if err != nil {
			continue
		}

		parsedLink, err := url.Parse(rawLink)
		if err != nil {
			continue
		}

		finalLink := parsedBase.ResolveReference(parsedLink).String()

		if strings.HasPrefix(finalLink, "http://") || strings.HasPrefix(finalLink, "https://") {
			links = append(links, finalLink)
		}
	}

	return links
}

