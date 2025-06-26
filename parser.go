package main

import (
	"io"
	"net/http"
	"net/url"
	"strings"

	"github.com/temoto/robotstxt"
	"golang.org/x/net/html"
)

var robotsCache = make(map[string]*robotstxt.RobotsData)

func ExtractLinks(Pagehtml string, baseURL string) []string {
	r := strings.NewReader(Pagehtml)
	doc, err := html.Parse(r)
	if err != nil {
		return []string{}
	}
	var links []string
	var VisitLinks func(n *html.Node)
	VisitLinks = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "a" {
			for _, attr := range n.Attr {
				if attr.Key == "href" {
					base, baseErr := url.Parse(baseURL)
					href, hrefErr := url.Parse(attr.Val)
					if hrefErr == nil && baseErr == nil {
						absolute := base.ResolveReference(href)
						absolute.Fragment = "" 
						absolute.RawQuery = "" 
						clean := absolute.String()
						links = append(links, clean)
					}
				}
			}
		}
		if n.FirstChild != nil {
			VisitLinks(n.FirstChild)
		}
		if n.NextSibling != nil {
			VisitLinks(n.NextSibling)
		}
	}
	VisitLinks(doc)
	return links

}

func ExtractTitle(HtmlStr string, baseURL string) string {
	r := strings.NewReader(HtmlStr)
	doc, err := html.Parse(r)
	if err != nil {
		return ""
	}
	var title string
	var FindTitle func(n *html.Node)
	FindTitle = func(n *html.Node) {
		if n.Type == html.ElementNode && n.Data == "title" {
			if n.FirstChild != nil {
				title = n.FirstChild.Data
			}
			return
		}
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			FindTitle(c)
		}
	}
	FindTitle(doc)
	return title
}

func Respect(rawurl string) *robotstxt.RobotsData {
	parsedURL, err := url.Parse(rawurl)
	if err != nil {
		return nil
	}
	robotsURL := parsedURL.Scheme + "://" + parsedURL.Host + "/robots.txt"
	if cached, ok := robotsCache[parsedURL.Host]; ok {
		return cached
	}
	resp, err := http.Get(robotsURL)
	if err != nil {
		return nil
	}
	defer resp.Body.Close()
	body, err := io.ReadAll(resp.Body)
	if err != nil {
		return nil
	}
	robots, err := robotstxt.FromStatusAndBytes(resp.StatusCode, body)
	if err != nil {
		return nil
	}
	robotsCache[parsedURL.Host] = robots
	return robots
}

func AllowedByRobots(url string) bool {
	robots := Respect(url)
	if robots == nil {
		return true 
	}
	group := robots.FindGroup("SoulSearchBot")
	return group.Test(url)
}

