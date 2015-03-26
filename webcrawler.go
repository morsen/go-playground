// DISABLED: +build netgo

package main

import (
	"fmt"
	// "io/ioutil"
	"net/http"
	"net/url"
	"golang.org/x/net/html"
)

type Fetcher interface {
	// Fetch returns the body of URL and
	// a slice of URLs found on that page.
	Fetch(url string) (body string, urls []string, err error)
}

type CrawlState struct {
	UrlsCrawled map[string]bool
	CrawlsInProgress int
}

type InitiateCrawlData struct {
	Fetcher Fetcher
	Url string
	Depth int
}

// Crawl uses fetcher to get the contents and branching urls from the given url. For each branch
// url, will reissue another crawl request on the crawlInitiatedChannel
func Crawl(initiateCrawlData *InitiateCrawlData, crawlInitiatedChannel chan InitiateCrawlData,
	crawlCompletedChannel chan string) {
	
	d := initiateCrawlData
	
	defer func() { crawlCompletedChannel <- d.Url }()
	
	// Note: We're throwing away the body because we don't care about it at this level
	_, newUrlsToCrawl, err := d.Fetcher.Fetch(d.Url)
	fmt.Print(".")
	if err != nil {
		fmt.Printf("\n%v\n", err)
		return
	}
	
	//fmt.Printf("found: %s\n", d.Url)
	
	for _, newUrlToCrawl := range newUrlsToCrawl {
		crawlInitiatedChannel <- InitiateCrawlData{ d.Fetcher, newUrlToCrawl, d.Depth - 1 }
	}
}

func ReceiveCrawls(crawlState CrawlState, crawlInitiatedChannel chan InitiateCrawlData,
	crawlCompletedChannel chan string) {
	
	defer func() {
		fmt.Println()
		for url, done := range crawlState.UrlsCrawled {
			fmt.Println(url)
			if !done {
				fmt.Printf("ERROR: Crawl not done for url = %v\n", url)
			}
		}
		fmt.Printf("Crawl Count = %v\n", len(crawlState.UrlsCrawled))
	}()
	
	for {
		
		select {
			
		case initiateCrawlData := <- crawlInitiatedChannel:
			
			d := &initiateCrawlData
			
			if _, exists := crawlState.UrlsCrawled[d.Url]; !exists && d.Depth > 0 {
			
				crawlState.UrlsCrawled[d.Url] = false
				crawlState.CrawlsInProgress += 1
			
				//fmt.Printf("Crawl initiated: d=%v, url='%v'\n", d.Depth, d.Url)
			
				go Crawl(d, crawlInitiatedChannel, crawlCompletedChannel)
			}		
			
		case urlCrawled := <- crawlCompletedChannel:
			
			//fmt.Printf("Crawl completed: %v\n", urlCrawled)
			
			if done, exists := crawlState.UrlsCrawled[urlCrawled]; done || !exists {
				fmt.Printf("\nCrawl result came back for one that's not in progress! url = %v\n", urlCrawled)
			} else {
				
				crawlState.UrlsCrawled[urlCrawled] = true
				crawlState.CrawlsInProgress -= 1
				
				if crawlState.CrawlsInProgress == 0 {
					return
				}
			}
		}
	}
}

func ConcurrentCrawler(initiateCrawlData InitiateCrawlData) {	
	
	crawlState := CrawlState{ make(map[string]bool), 0 }
	crawlInitiatedChannel := make(chan InitiateCrawlData)
	crawlCompletedChannel := make(chan string)
	
	go func() { crawlInitiatedChannel <- initiateCrawlData }()
	
	ReceiveCrawls(crawlState, crawlInitiatedChannel, crawlCompletedChannel)
}

func main() {	
	//ConcurrentCrawler(InitiateCrawlData{ fakeFetcher, "http://golang.org/", 5 })
	ConcurrentCrawler(InitiateCrawlData{ RealFetcher{}, "http://www.cnn.com/", 2})
	//ConcurrentCrawler(InitiateCrawlData{ RealFetcher{}, "http://www.salon.com/", 2})
}

type RealFetcher struct {}

func (fetcher RealFetcher) Fetch(urlToFetch string) (string, []string, error) {

	parsedUrlToFetch, err := url.Parse(urlToFetch)
	if err != nil {
		return "", nil, err
	}
	
	resp, err := http.Get(parsedUrlToFetch.String())
	if err != nil {
		return "", nil, err
	}
	
	/*
	body, err := ioutil.ReadAll(resp.Body)
	if err != nil {
		return "", nil, err
	}	
	fmt.Println(string(body))
	*/
	body := ""
	
	dom, err := html.Parse(resp.Body)
	if err != nil {
		resp.Body.Close()
		return "", nil, err
	}
	resp.Body.Close()
	
	urls := make([]string, 0)
	
	var walkDom func(*html.Node)
	walkDom = func(n *html.Node) {
	
		if n.Type == html.ElementNode && n.Data == "a" { // TODO: Check for 'A' also?
			for _, v := range n.Attr {
				if v.Key == "href" {
					newUrl, err := url.Parse(v.Val)
					if err != nil {
						fmt.Printf("Couldn't parse URL from response body: '%v'\n", v.Val)
					} else {
						newUrl = parsedUrlToFetch.ResolveReference(newUrl) // Make relative URLs absolute
						urls = append(urls, newUrl.String())
					}
				}
			}
		}
		
		for c := n.FirstChild; c != nil; c = c.NextSibling {
			walkDom(c)
		}
	}
	
	walkDom(dom)
	
	return string(body), urls, nil
}

// fakeFetcher is Fetcher that returns canned results.
type FakeFetcher map[string]*FakeResult

type FakeResult struct {
	body string
	urls []string
}

func (f FakeFetcher) Fetch(url string) (string, []string, error) {
	if res, ok := f[url]; ok {
		return res.body, res.urls, nil
	}
	return "", nil, fmt.Errorf("not found: %s", url)
}

// fetcher is a populated fakeFetcher.
var fakeFetcher = FakeFetcher{
	"http://golang.org/": &FakeResult{
		"The Go Programming Language",
		[]string{
			"http://golang.org/pkg/",
			"http://golang.org/cmd/",
		},
	},
	"http://golang.org/pkg/": &FakeResult{
		"Packages",
		[]string{
			"http://golang.org/",
			"http://golang.org/cmd/",
			"http://golang.org/pkg/fmt/",
			"http://golang.org/pkg/os/",
		},
	},
	"http://golang.org/pkg/fmt/": &FakeResult{
		"Package fmt",
		[]string{
			"http://golang.org/",
			"http://golang.org/pkg/",
		},
	},
	"http://golang.org/pkg/os/": &FakeResult{
		"Package os",
		[]string{
			"http://golang.org/",
			"http://golang.org/pkg/",
		},
	},
}

